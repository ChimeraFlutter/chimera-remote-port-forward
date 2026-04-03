package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
)

// WebServer Web管理界面服务
type WebServer struct {
	server   *Server
	sessions map[string]time.Time // session -> 过期时间
	mu       sync.RWMutex
	logger   *logger.Logger
}

// NewWebServer 创建Web服务
func NewWebServer(server *Server, logger *logger.Logger) *WebServer {
	return &WebServer{
		server:   server,
		sessions: make(map[string]time.Time),
		logger:   logger,
	}
}

// Start 启动Web服务
func (w *WebServer) Start(addr string) error {
	// 路由
	http.HandleFunc("/", w.handleIndex)
	http.HandleFunc("/api/login", w.handleLogin)
	http.HandleFunc("/api/devices", w.handleDevices)
	http.HandleFunc("/api/device/enable", w.handleDeviceEnable)
	http.HandleFunc("/api/device/disable", w.handleDeviceDisable)

	w.logger.Info("Web interface started",
		logger.String("addr", addr))
	return http.ListenAndServe(addr, nil)
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
    <title>Login - Chimera Port Forward</title>
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
        <h1>Chimera Port Forward</h1>
        <div class="form-group">
            <label>Password</label>
            <input type="password" id="password" placeholder="Enter password">
        </div>
        <button onclick="login()">Login</button>
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
            .then(function(res) { return res.json(); })
            .then(function(data) {
                localStorage.setItem('token', data.token);
                window.location.href = '/';
            })
            .catch(function(err) {
                errorDiv.textContent = 'Login failed';
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
    <title>Chimera Port Forward</title>
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
        <h1>Chimera Port Forward</h1>
        <button class="logout" onclick="logout()">Logout</button>
    </div>
    <div class="container">
        <table>
            <thead>
                <tr>
                    <th>Device Name</th>
                    <th>Local Port</th>
                    <th>Remote Port</th>
                    <th>Status</th>
                    <th>Remaining Time</th>
                    <th>Last Heartbeat</th>
                    <th>Action</th>
                </tr>
            </thead>
            <tbody id="deviceList"></tbody>
        </table>
        <div id="empty" class="empty" style="display: none;">No devices connected</div>
    </div>

    <!-- Enable Modal -->
    <div id="enableModal" class="modal">
        <div class="modal-content">
            <div class="modal-title">Enable Device: <span id="modalDeviceName"></span></div>
            <div class="duration-options">
                <button class="duration-btn" onclick="confirmEnable(1)">1 Hour</button>
                <button class="duration-btn" onclick="confirmEnable(6)">6 Hours</button>
                <button class="duration-btn" onclick="confirmEnable(12)">12 Hours</button>
                <button class="duration-btn" onclick="confirmEnable(24)">24 Hours (Default)</button>
                <button class="duration-btn" onclick="confirmEnable(0)">Permanent (No Expiry)</button>
            </div>
            <button class="modal-cancel" onclick="closeModal()">Cancel</button>
        </div>
    </div>

    <script>
        var token = localStorage.getItem('token');
        if (!token) {
            window.location.href = '/';
        }

        var pendingDevice = null;
        var countdownTimers = {};

        function logout() {
            localStorage.removeItem('token');
            window.location.href = '/';
        }

        function formatRemaining(seconds) {
            if (seconds <= 0) return 'Expired';
            var h = Math.floor(seconds / 3600);
            var m = Math.floor((seconds % 3600) / 60);
            var s = seconds % 60;
            return h + 'h ' + m + 'm ' + s + 's';
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

        function loadDevices() {
            fetch('/api/devices', {
                headers: { 'Authorization': 'Bearer ' + token }
            })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                var tbody = document.getElementById('deviceList');
                var empty = document.getElementById('empty');

                if (data.devices.length === 0) {
                    tbody.innerHTML = '';
                    empty.style.display = 'block';
                    return;
                }

                empty.style.display = 'none';
                var html = '';
                data.devices.forEach(function(d) {
                    var statusClass = d.enabled ? 'status-online' : 'status-disabled';
                    var statusText = d.enabled ? 'Enabled' : 'Disabled';

                    var remainingHtml = '-';
                    if (d.enabled) {
                        if (d.remaining_seconds > 0) {
                            remainingHtml = '<span class="remaining-time" data-remaining="' + d.remaining_seconds + '">' + formatRemaining(d.remaining_seconds) + '</span>';
                        } else if (d.expire_at === '') {
                            remainingHtml = '<span class="remaining-time">Permanent</span>';
                        }
                    }

                    var actionHtml = '';
                    if (d.enabled) {
                        actionHtml = '<button class="btn-disable" onclick="disableDevice(\'' + d.name + '\')">Disable</button>';
                    } else {
                        actionHtml = '<button class="btn-enable" onclick="showEnableModal(\'' + d.name + '\')">Enable</button>';
                    }

                    html += '<tr>' +
                        '<td>' + d.name + '</td>' +
                        '<td>' + d.local_port + '</td>' +
                        '<td>' + d.remote_port + '</td>' +
                        '<td class="' + statusClass + '">' + statusText + '</td>' +
                        '<td>' + remainingHtml + '</td>' +
                        '<td>' + d.last_heartbeat + '</td>' +
                        '<td>' + actionHtml + '</td>' +
                        '</tr>';
                });
                tbody.innerHTML = html;
            })
            .catch(function(err) {
                console.error('Failed to load devices:', err);
            });
        }

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
                    alert('Error: ' + data.error);
                } else {
                    closeModal();
                    loadDevices();
                }
            })
            .catch(function(err) {
                alert('Failed to enable device');
            });
        }

        function disableDevice(deviceName) {
            if (!confirm('Disable device ' + deviceName + '?')) return;

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
                    alert('Error: ' + data.error);
                } else {
                    loadDevices();
                }
            })
            .catch(function(err) {
                alert('Failed to disable device');
            });
        }

        loadDevices();
        setInterval(loadDevices, 5000);
        setInterval(updateCountdowns, 1000);
    </script>
</body>
</html>`
