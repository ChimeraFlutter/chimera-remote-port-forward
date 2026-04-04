package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
	"github.com/gorilla/websocket"
)

// WebServer Web管理界面服务
type WebServer struct {
	server       *Server
	sessions     map[string]time.Time // session -> 过期时间
	adminClients map[*websocket.Conn]bool // 管理界面的 WebSocket 客户端
	mu           sync.RWMutex
	logger       *logger.Logger
	upgrader     websocket.Upgrader
	httpSrv      *http.Server
}

// NewWebServer 创建Web服务
func NewWebServer(server *Server, log *logger.Logger) *WebServer {
	return &WebServer{
		server:       server,
		sessions:     make(map[string]time.Time),
		adminClients: make(map[*websocket.Conn]bool),
		logger:       log,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// Start 启动Web服务
func (w *WebServer) Start(addr string) error {
	// 路由
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.handleIndex)
	mux.HandleFunc("/api/login", w.handleLogin)
	mux.HandleFunc("/api/devices", w.handleDevices)
	mux.HandleFunc("/api/device/enable", w.handleDeviceEnable)
	mux.HandleFunc("/api/device/disable", w.handleDeviceDisable)
	mux.HandleFunc("/ws/admin", w.handleAdminWS) // 管理界面 WebSocket

	w.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	w.logger.Info("Web interface started",
		logger.String("addr", addr))
	return w.httpSrv.ListenAndServe()
}

// Stop 停止Web服务
func (w *WebServer) Stop() {
	w.logger.Info("WebServer.Stop: collecting admin connections")
	// 收集要关闭的连接，在锁外关闭以避免死锁
	w.mu.Lock()
	var connsToClose []*websocket.Conn
	for conn := range w.adminClients {
		connsToClose = append(connsToClose, conn)
	}
	w.adminClients = make(map[*websocket.Conn]bool)
	w.mu.Unlock()

	w.logger.Info("WebServer.Stop: closing admin connections")
	for _, conn := range connsToClose {
		conn.Close()
	}
	w.logger.Info("WebServer.Stop: admin connections closed")

	// 再关闭 HTTP 服务器
	if w.httpSrv != nil {
		w.logger.Info("WebServer.Stop: shutting down HTTP")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		w.httpSrv.Shutdown(ctx)
		w.logger.Info("WebServer.Stop: HTTP shutdown complete")
	}
}

// handleIndex 处理首页
func (w *WebServer) handleIndex(rw http.ResponseWriter, r *http.Request) {
	// 验证session
	if !w.authenticate(r) {
		w.renderLogin(rw)
		return
	}

	// 渲染设备列表页面
	w.renderDevices(rw)
}

// handleLogin 处理登录
func (w *WebServer) handleLogin(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request", http.StatusBadRequest)
		return
	}

	// 验证密码
	if req.Password != w.server.config.WebPassword {
		http.Error(rw, "Invalid password", http.StatusUnauthorized)
		return
	}

	// 生成session token
	session := w.generateSession()

	w.mu.Lock()
	w.sessions[session] = time.Now().Add(24 * time.Hour) // 24小时过期
	w.mu.Unlock()

	// 设置 cookie (24小时过期)
	http.SetCookie(rw, &http.Cookie{
		Name:     "token",
		Value:    session,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400, // 24小时
	})

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"token": session})
}

// handleDevices 处理获取设备列表
func (w *WebServer) handleDevices(rw http.ResponseWriter, r *http.Request) {
	// 验证session
	if !w.authenticate(r) {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	devices := w.server.GetDevices()

	// 构建响应数据
	type DeviceInfo struct {
		Name             string `json:"name"`
		LocalIP          string `json:"local_ip"`
		LocalPort        int    `json:"local_port"`
		RemotePort       int    `json:"remote_port"`
		Status           string `json:"status"`
		LastHeartbeat    string `json:"last_heartbeat"`
		Connections      int    `json:"connections"`
		Enabled          bool   `json:"enabled"`
		ExpireAt         string `json:"expire_at"`
		RemainingSeconds int64  `json:"remaining_seconds"`
	}

	deviceInfos := make([]DeviceInfo, 0, len(devices))
	now := time.Now()
	for _, d := range devices {
		info := DeviceInfo{
			Name:          d.Name,
			LocalIP:       d.LocalIP,
			LocalPort:     d.LocalPort,
			RemotePort:    d.RemotePort,
			Status:        "online",
			LastHeartbeat: d.LastHeartbeat.Format("2006-01-02 15:04:05"),
			Connections:   0, // TODO: 从proxy获取连接数
			Enabled:       d.Enabled,
		}

		if !d.ExpireAt.IsZero() {
			info.ExpireAt = d.ExpireAt.Format("2006-01-02 15:04:05")
			remaining := d.ExpireAt.Sub(now)
			if remaining > 0 {
				info.RemainingSeconds = int64(remaining.Seconds())
			}
		}

		deviceInfos = append(deviceInfos, info)
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]interface{}{
		"devices": deviceInfos,
	})
}

// handleDeviceEnable 处理启用设备
func (w *WebServer) handleDeviceEnable(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 验证session
	if !w.authenticate(r) {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		DeviceName    string `json:"device_name"`
		DurationHours int    `json:"duration_hours"` // 0 表示永久
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request", http.StatusBadRequest)
		return
	}

	var duration time.Duration
	if req.DurationHours > 0 {
		duration = time.Duration(req.DurationHours) * time.Hour
	}

	if err := w.server.EnableDevice(req.DeviceName, duration); err != nil {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(map[string]string{"error": err.Error()})
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"status": "ok"})
}

// handleDeviceDisable 处理禁用设备
func (w *WebServer) handleDeviceDisable(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 验证session
	if !w.authenticate(r) {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		DeviceName string `json:"device_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := w.server.DisableDevice(req.DeviceName); err != nil {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(map[string]string{"error": err.Error()})
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(map[string]string{"status": "ok"})
}

// authenticate 验证session
func (w *WebServer) authenticate(r *http.Request) bool {
	token := r.Header.Get("Authorization")
	if token == "" {
		// 尝试从 Cookie 获取
		cookie, err := r.Cookie("token")
		if err == nil {
			token = cookie.Value
		}
	}

	if token == "" {
		return false
	}

	// 移除 "Bearer " 前缀
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	expiry, exists := w.sessions[token]
	if !exists {
		return false
	}

	// 检查是否过期
	if time.Now().After(expiry) {
		return false
	}

	return true
}

// generateSession 生成session token
func (w *WebServer) generateSession() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 回退到时间戳，但这种情况极少发生
		return time.Now().Format("20060102150405.000")
	}
	return base64.URLEncoding.EncodeToString(b)
}

// handleAdminWS 处理管理界面 WebSocket 连接
func (w *WebServer) handleAdminWS(rw http.ResponseWriter, r *http.Request) {
	// 验证 token
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
	}

	w.mu.RLock()
	expiry, exists := w.sessions[token]
	valid := exists && time.Now().Before(expiry)
	w.mu.RUnlock()

	if !valid {
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 升级为 WebSocket
	conn, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		w.logger.Error("WebSocket upgrade failed", logger.Err(err))
		return
	}
	defer conn.Close()

	// 注册客户端
	w.mu.Lock()
	w.adminClients[conn] = true
	w.mu.Unlock()

	w.logger.Info("Admin WebSocket connected")

	// 清理函数
	defer func() {
		w.mu.Lock()
		delete(w.adminClients, conn)
		w.mu.Unlock()
		w.logger.Info("Admin WebSocket disconnected")
	}()

	// 立即发送当前设备状态
	w.sendDevicesToClient(conn)

	// 保持连接，读取客户端消息
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// BroadcastDevices 广播设备状态变更给所有管理客户端
func (w *WebServer) BroadcastDevices() {
	w.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(w.adminClients))
	for conn := range w.adminClients {
		clients = append(clients, conn)
	}
	w.mu.RUnlock()

	for _, conn := range clients {
		w.sendDevicesToClient(conn)
	}
}

// sendDevicesToClient 发送设备状态给单个客户端
func (w *WebServer) sendDevicesToClient(conn *websocket.Conn) {
	devices := w.server.GetDevices()

	// 构建响应数据
	type DeviceInfo struct {
		Name             string `json:"name"`
		LocalIP          string `json:"local_ip"`
		LocalPort        int    `json:"local_port"`
		RemotePort       int    `json:"remote_port"`
		Status           string `json:"status"`
		LastHeartbeat    string `json:"last_heartbeat"`
		Connections      int    `json:"connections"`
		Enabled          bool   `json:"enabled"`
		ExpireAt         string `json:"expire_at"`
		RemainingSeconds int64  `json:"remaining_seconds"`
	}

	deviceInfos := make([]DeviceInfo, 0, len(devices))
	now := time.Now()
	for _, d := range devices {
		info := DeviceInfo{
			Name:          d.Name,
			LocalIP:       d.LocalIP,
			LocalPort:     d.LocalPort,
			RemotePort:    d.RemotePort,
			Status:        "online",
			LastHeartbeat: d.LastHeartbeat.Format("2006-01-02 15:04:05"),
			Connections:   0,
			Enabled:       d.Enabled,
		}

		if !d.ExpireAt.IsZero() {
			info.ExpireAt = d.ExpireAt.Format("2006-01-02 15:04:05")
			remaining := d.ExpireAt.Sub(now)
			if remaining > 0 {
				info.RemainingSeconds = int64(remaining.Seconds())
			}
		}

		deviceInfos = append(deviceInfos, info)
	}

	msg := map[string]interface{}{
		"type":    "devices",
		"devices": deviceInfos,
	}

	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)
}

// renderLogin 渲染登录页面
func (w *WebServer) renderLogin(rw http.ResponseWriter) {
	tmpl, err := template.New("login").Parse(loginHTML)
	if err != nil {
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html")
	tmpl.Execute(rw, nil)
}

// renderDevices 渲染设备列表页面
func (w *WebServer) renderDevices(rw http.ResponseWriter) {
	tmpl, err := template.New("devices").Parse(devicesHTML)
	if err != nil {
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html")
	tmpl.Execute(rw, nil)
}

const loginHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>登录 - Chimera 端口转发</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .login-box {
            background: white;
            padding: 40px;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.1);
            width: 100%;
            max-width: 400px;
        }
        h1 {
            text-align: center;
            margin-bottom: 30px;
            color: #333;
        }
        .form-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            margin-bottom: 8px;
            color: #666;
        }
        input[type="password"] {
            width: 100%;
            padding: 12px;
            border: 1px solid #ddd;
            border-radius: 5px;
            font-size: 16px;
        }
        button {
            width: 100%;
            padding: 12px;
            background: #667eea;
            color: white;
            border: none;
            border-radius: 5px;
            font-size: 16px;
            cursor: pointer;
            transition: background 0.3s;
        }
        button:hover {
            background: #5568d3;
        }
        .error {
            color: #e74c3c;
            margin-top: 10px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="login-box">
        <h1>Chimera 端口转发</h1>
        <div class="form-group">
            <label>密码</label>
            <input type="password" id="password" placeholder="请输入密码">
        </div>
        <button onclick="login()">登录</button>
        <div id="error" class="error"></div>
    </div>
    <script>
        function login() {
            var password = document.getElementById('password').value;
            var errorDiv = document.getElementById('error');

            fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password: password })
            })
            .then(function(res) {
                if (!res.ok) {
                    return res.json().then(function(data) {
                        throw new Error(data.error || '密码错误');
                    });
                }
                return res.json();
            })
            .then(function(data) {
                localStorage.setItem('token', data.token);
                window.location.reload();
            })
            .catch(function(err) {
                errorDiv.textContent = err.message || '登录失败';
            });
        }

        document.getElementById('password').addEventListener('keypress', function(e) {
            if (e.key === 'Enter') login();
        });
    </script>
</body>
</html>`

const devicesHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Chimera 端口转发</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f7fa;
            padding: 20px;
        }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 30px;
        }
        h1 { color: #333; }
        .logout {
            padding: 8px 16px;
            background: #e74c3c;
            color: white;
            border: none;
            border-radius: 5px;
            cursor: pointer;
        }
        .container {
            background: white;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.05);
            overflow: hidden;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 15px;
            text-align: left;
            border-bottom: 1px solid #eee;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
            color: #666;
        }
        .status-online {
            color: #27ae60;
            font-weight: 600;
        }
        .status-offline {
            color: #e74c3c;
            font-weight: 600;
        }
        .status-disabled {
            color: #95a5a6;
            font-weight: 600;
        }
        .empty {
            text-align: center;
            padding: 40px;
            color: #999;
        }
        .btn-enable {
            padding: 6px 12px;
            background: #27ae60;
            color: white;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 13px;
        }
        .btn-enable:hover {
            background: #219a52;
        }
        .btn-disable {
            padding: 6px 12px;
            background: #e74c3c;
            color: white;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 13px;
        }
        .btn-disable:hover {
            background: #c0392b;
        }
        .remaining-time {
            font-family: monospace;
            color: #666;
        }
        .remaining-time.warning {
            color: #e74c3c;
        }
        /* Modal */
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            align-items: center;
            justify-content: center;
        }
        .modal.show {
            display: flex;
        }
        .modal-content {
            background: white;
            padding: 30px;
            border-radius: 10px;
            width: 100%;
            max-width: 400px;
        }
        .modal-title {
            font-size: 18px;
            font-weight: 600;
            margin-bottom: 20px;
        }
        .duration-options {
            display: flex;
            flex-direction: column;
            gap: 10px;
        }
        .duration-btn {
            padding: 12px;
            background: #f8f9fa;
            border: 1px solid #ddd;
            border-radius: 5px;
            cursor: pointer;
            text-align: left;
            font-size: 14px;
        }
        .duration-btn:hover {
            background: #667eea;
            color: white;
            border-color: #667eea;
        }
        .modal-cancel {
            margin-top: 15px;
            padding: 10px;
            background: #e74c3c;
            color: white;
            border: none;
            border-radius: 5px;
            cursor: pointer;
            width: 100%;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>Chimera 端口转发</h1>
        <button class="logout" onclick="logout()">退出</button>
    </div>
    <div class="container">
        <table>
            <thead>
                <tr>
                    <th>设备名称</th>
                    <th>本地地址</th>
                    <th>远程端口</th>
                    <th>状态</th>
                    <th>剩余时间</th>
                    <th>最后心跳</th>
                    <th>操作</th>
                </tr>
            </thead>
            <tbody id="deviceList"></tbody>
        </table>
        <div id="empty" class="empty" style="display: none;">暂无设备连接</div>
    </div>

    <!-- Enable Modal -->
    <div id="enableModal" class="modal">
        <div class="modal-content">
            <div class="modal-title">启用设备: <span id="modalDeviceName"></span></div>
            <div class="duration-options">
                <button class="duration-btn" onclick="confirmEnable(1)">1 小时</button>
                <button class="duration-btn" onclick="confirmEnable(6)">6 小时</button>
                <button class="duration-btn" onclick="confirmEnable(12)">12 小时</button>
                <button class="duration-btn" onclick="confirmEnable(24)">24 小时 (默认)</button>
                <button class="duration-btn" onclick="confirmEnable(0)">永久 (不过期)</button>
            </div>
            <button class="modal-cancel" onclick="closeModal()">取消</button>
        </div>
    </div>

    <script>
        var token = localStorage.getItem('token');
        if (!token) {
            window.location.href = '/';
        }

        var pendingDevice = null;
        var ws = null;
        var reconnectTimer = null;

        function logout() {
            localStorage.removeItem('token');
            if (ws) ws.close();
            window.location.href = '/';
        }

        function formatRemaining(seconds) {
            if (seconds <= 0) return '已过期';
            var h = Math.floor(seconds / 3600);
            var m = Math.floor((seconds % 3600) / 60);
            var s = seconds % 60;
            return h + '时 ' + m + '分 ' + s + '秒';
        }

        function updateCountdowns() {
            var elements = document.querySelectorAll('[data-remaining]');
            elements.forEach(function(el) {
                var remaining = parseInt(el.getAttribute('data-remaining'));
                if (remaining > 0) {
                    remaining--;
                    el.setAttribute('data-remaining', remaining);
                    el.textContent = formatRemaining(remaining);
                    if (remaining < 3600) {
                        el.classList.add('warning');
                    }
                }
            });
        }

        function renderDevices(devices) {
            var tbody = document.getElementById('deviceList');
            var empty = document.getElementById('empty');

            if (devices.length === 0) {
                tbody.innerHTML = '';
                empty.style.display = 'block';
                return;
            }

            empty.style.display = 'none';
            var html = '';
            devices.forEach(function(d) {
                var statusClass = d.enabled ? 'status-online' : 'status-disabled';
                var statusText = d.enabled ? '已启用' : '已禁用';

                var remainingHtml = '-';
                if (d.enabled) {
                    if (d.remaining_seconds > 0) {
                        remainingHtml = '<span class="remaining-time" data-remaining="' + d.remaining_seconds + '">' + formatRemaining(d.remaining_seconds) + '</span>';
                    } else if (d.expire_at === '') {
                        remainingHtml = '<span class="remaining-time">永久</span>';
                    }
                }

                var actionHtml = '';
                if (d.enabled) {
                    actionHtml = '<button class="btn-disable" onclick="disableDevice(\'' + d.name + '\')">禁用</button>';
                } else {
                    actionHtml = '<button class="btn-enable" onclick="showEnableModal(\'' + d.name + '\')">启用</button>';
                }

                var localAddr = (d.local_ip || '127.0.0.1') + ':' + d.local_port;
                html += '<tr>' +
                    '<td>' + d.name + '</td>' +
                    '<td>' + localAddr + '</td>' +
                    '<td>' + d.remote_port + '</td>' +
                    '<td class="' + statusClass + '">' + statusText + '</td>' +
                    '<td>' + remainingHtml + '</td>' +
                    '<td>' + d.last_heartbeat + '</td>' +
                    '<td>' + actionHtml + '</td>' +
                    '</tr>';
            });
            tbody.innerHTML = html;
        }

        function connectWebSocket() {
            var wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            var wsUrl = wsProtocol + '//' + window.location.host + '/ws/admin?token=' + encodeURIComponent(token);

            ws = new WebSocket(wsUrl);

            ws.onopen = function() {
                console.log('WebSocket 已连接');
                if (reconnectTimer) {
                    clearTimeout(reconnectTimer);
                    reconnectTimer = null;
                }
            };

            ws.onmessage = function(event) {
                var data = JSON.parse(event.data);
                if (data.type === 'devices') {
                    renderDevices(data.devices);
                }
            };

            ws.onclose = function() {
                console.log('WebSocket 已断开，5秒后重连...');
                ws = null;
                reconnectTimer = setTimeout(connectWebSocket, 5000);
            };

            ws.onerror = function(err) {
                console.error('WebSocket 错误:', err);
            };
        }

        // 启动 WebSocket 连接
        connectWebSocket();

        // 每秒更新倒计时显示
        setInterval(updateCountdowns, 1000);

        function showEnableModal(deviceName) {
            pendingDevice = deviceName;
            document.getElementById('modalDeviceName').textContent = deviceName;
            document.getElementById('enableModal').classList.add('show');
        }

        function closeModal() {
            document.getElementById('enableModal').classList.remove('show');
            pendingDevice = null;
        }

        function confirmEnable(hours) {
            if (!pendingDevice) return;

            fetch('/api/device/enable', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + token
                },
                body: JSON.stringify({
                    device_name: pendingDevice,
                    duration_hours: hours
                })
            })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (data.error) {
                    alert('错误: ' + data.error);
                } else {
                    closeModal();
                    // WebSocket 会自动推送更新，无需手动刷新
                }
            })
            .catch(function(err) {
                alert('启用设备失败');
            });
        }

        function disableDevice(deviceName) {
            if (!confirm('确定要禁用设备 ' + deviceName + ' 吗？')) return;

            fetch('/api/device/disable', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + token
                },
                body: JSON.stringify({ device_name: deviceName })
            })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (data.error) {
                    alert('错误: ' + data.error);
                }
                // WebSocket 会自动推送更新，无需手动刷新
            })
            .catch(function(err) {
                alert('禁用设备失败');
            });
        }
    </script>
</body>
</html>`
