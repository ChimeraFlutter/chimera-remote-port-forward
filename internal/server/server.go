package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chimera/chimera-remote-port-forward/internal/protocol"
	"github.com/chimera/chimera-remote-port-forward/pkg/config"
	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
	"github.com/gorilla/websocket"
)

// Device 设备信息
type Device struct {
	Name          string          // 设备名称
	LocalIP       string          // 本地IP
	LocalPort     int             // 本地端口
	RemotePort    int             // 远程端口
	Conn          *websocket.Conn // WebSocket连接
	LastHeartbeat time.Time       // 最后心跳时间
	connMu        sync.Mutex      // 保护WebSocket写入
	Enabled       bool            // 是否已启用
	ExpireAt      time.Time       // 过期时间（零值表示永久）
}

// Server 服务端
type Server struct {
	config   *config.ServerConfig
	portPool *PortPool
	devices  map[string]*Device           // 设备名 -> 设备
	conns    map[*websocket.Conn]*Device  // 连接 -> 设备
	proxies  map[int]*Proxy               // 端口 -> 代理
	mu       sync.RWMutex
	upgrader websocket.Upgrader
	logger   *logger.Logger
	httpSrv  *http.Server
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewServer 创建服务端
func NewServer(cfg *config.ServerConfig, log *logger.Logger) *Server {
	if cfg == nil {
		cfg = config.DefaultServerConfig()
	}

	return &Server{
		config:   cfg,
		portPool: NewPortPool(cfg.PortStart, cfg.PortEnd),
		devices:  make(map[string]*Device),
		conns:    make(map[*websocket.Conn]*Device),
		proxies:  make(map[int]*Proxy),
		logger:   log,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// Start 启动服务端
func (s *Server) Start() error {
	// 启动心跳检测
	go s.checkHeartbeatTimeout()

	// 启动过期设备检查
	go s.checkExpiredDevices()

	// 启动Web管理界面
	webServer := NewWebServer(s, s.logger)
	go func() {
		if err := webServer.Start(s.config.Web); err != nil {
			s.logger.Error("Web server failed", logger.Err(err))
		}
	}()

	// 设置路由
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	s.httpSrv = &http.Server{
		Addr:    s.config.Listen,
		Handler: mux,
	}

	s.logger.Info("WebSocket listening",
		logger.String("addr", s.config.Listen),
		logger.Int("port_start", s.config.PortStart),
		logger.Int("port_end", s.config.PortEnd))

	return s.httpSrv.ListenAndServe()
}

// Stop 停止服务端
func (s *Server) Stop() {
	close(s.stopCh)

	s.mu.Lock()
	// 停止所有代理
	for port, proxy := range s.proxies {
		proxy.Stop()
		delete(s.proxies, port)
	}
	// 收集要关闭的连接，在锁外关闭以避免死锁
	var connsToClose []*websocket.Conn
	for _, device := range s.devices {
		connsToClose = append(connsToClose, device.Conn)
	}
	s.devices = make(map[string]*Device)
	s.conns = make(map[*websocket.Conn]*Device)
	s.mu.Unlock()

	// 在锁外关闭连接
	for _, conn := range connsToClose {
		conn.Close()
	}

	// 关闭HTTP服务器
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			s.logger.Error("HTTP server shutdown error", logger.Err(err))
		}
	}

	close(s.doneCh)
	s.logger.Info("Server stopped")
}

// handleWebSocket 处理WebSocket连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", logger.Err(err))
		return
	}
	defer conn.Close()

	s.logger.Info("New WebSocket connection",
		logger.String("client_addr", conn.RemoteAddr().String()))

	// 消息处理循环
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			s.logger.Info("WebSocket connection closed",
				logger.String("client_addr", conn.RemoteAddr().String()),
				logger.Err(err))
			s.handleDisconnect(conn)
			break
		}

		var msg protocol.ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			s.logger.Error("Invalid message format", logger.Err(err))
			s.sendError(conn, "invalid message format")
			continue
		}

		switch msg.Type {
		case protocol.TypeRegister:
			s.logger.Info("Received register message",
				logger.String("device", msg.DeviceName))
			s.handleRegister(conn, &msg)
		case protocol.TypeHeartbeat:
			s.handleHeartbeat(conn)
		case protocol.TypeData:
			s.logger.Info("Received data message from client",
				logger.String("conn_id", msg.ConnID),
				logger.Int("bytes", len(msg.Data)))
			s.handleData(conn, &msg)
		case protocol.TypeConnClose:
			s.logger.Info("Received conn_close message from client",
				logger.String("conn_id", msg.ConnID))
			s.handleConnCloseFromClient(conn, &msg)
		default:
			s.logger.Warn("Unknown message type received",
				logger.String("type", msg.Type))
		}
	}
}

// handleRegister 处理注册消息
func (s *Server) handleRegister(conn *websocket.Conn, msg *protocol.ClientMessage) {
	// 验证Token
	if s.config.AuthToken != "" && msg.Token != s.config.AuthToken {
		s.logger.Warn("Authentication failed",
			logger.String("device", msg.DeviceName))
		s.sendError(conn, "authentication failed")
		conn.Close()
		return
	}

	// 验证必要字段
	if msg.DeviceName == "" {
		s.sendError(conn, "device name required")
		return
	}
	if msg.LocalPort <= 0 {
		s.sendError(conn, "local port required")
		return
	}

	// 检查设备数限制
	s.mu.RLock()
	currentDeviceCount := len(s.devices)
	s.mu.RUnlock()
	if s.config.MaxDevices > 0 && currentDeviceCount >= s.config.MaxDevices {
		s.sendError(conn, "max devices limit reached")
		conn.Close()
		return
	}

	// 检查设备名是否已存在
	s.mu.Lock()
	if _, exists := s.devices[msg.DeviceName]; exists {
		s.mu.Unlock()
		s.sendError(conn, "device name already exists")
		return
	}

	// 分配端口
	device := &Device{
		Name:          msg.DeviceName,
		LocalIP:       msg.LocalIP,
		LocalPort:     msg.LocalPort,
		Conn:          conn,
		LastHeartbeat: time.Now(),
		Enabled:       false, // 默认未启用，需在 Web 界面手动开启
	}

	port, err := s.portPool.Allocate(device)
	if err != nil {
		s.mu.Unlock()
		s.sendError(conn, err.Error())
		return
	}

	device.RemotePort = port
	s.devices[msg.DeviceName] = device
	s.conns[conn] = device

	// 不再自动启动代理，需在 Web 界面手动启用
	// proxy := NewProxy(port, device, s.config.MaxConnsPerProxy, s.logger)
	// ...

	s.mu.Unlock()

	s.logger.Info("Device registered",
		logger.String("device", msg.DeviceName),
		logger.Int("local_port", msg.LocalPort),
		logger.Int("remote_port", port))

	// 发送分配的端口
	s.sendMessage(conn, &protocol.ServerMessage{
		Type:       protocol.TypeAssigned,
		RemotePort: port,
	})
}

// handleHeartbeat 处理心跳消息
func (s *Server) handleHeartbeat(conn *websocket.Conn) {
	s.mu.RLock()
	device, exists := s.conns[conn]
	s.mu.RUnlock()

	if exists {
		device.connMu.Lock()
		device.LastHeartbeat = time.Now()
		data, err := json.Marshal(&protocol.ServerMessage{Type: protocol.TypeHeartbeatAck})
		if err == nil {
			conn.WriteMessage(websocket.TextMessage, data)
		}
		device.connMu.Unlock()
	}
}

// handleData 处理数据消息
func (s *Server) handleData(conn *websocket.Conn, msg *protocol.ClientMessage) {
	s.mu.RLock()
	device, exists := s.conns[conn]
	s.mu.RUnlock()

	if !exists {
		s.logger.Warn("handleData: device not found for WebSocket connection",
			logger.String("conn_addr", conn.RemoteAddr().String()),
			logger.String("conn_id", msg.ConnID),
			logger.Int("data_bytes", len(msg.Data)))
		return
	}

	// 转发数据到代理
	if proxy, ok := s.proxies[device.RemotePort]; ok {
		s.logger.Info("Server forwarding data from client to proxy",
			logger.String("device", device.Name),
			logger.String("conn_id", msg.ConnID),
			logger.Int("bytes", len(msg.Data)))
		proxy.HandleFromClient(msg.ConnID, msg.Data)
	} else {
		s.logger.Warn("handleData: no proxy found for device",
			logger.String("device", device.Name),
			logger.Int("remote_port", device.RemotePort),
			logger.Bool("enabled", device.Enabled),
			logger.String("conn_id", msg.ConnID))
	}
}

// handleConnCloseFromClient 处理客户端发来的连接关闭消息
func (s *Server) handleConnCloseFromClient(conn *websocket.Conn, msg *protocol.ClientMessage) {
	s.mu.RLock()
	device, exists := s.conns[conn]
	s.mu.RUnlock()

	if !exists {
		s.logger.Warn("handleConnCloseFromClient: device not found",
			logger.String("conn_id", msg.ConnID))
		return
	}

	// 关闭代理中的对应连接
	if proxy, ok := s.proxies[device.RemotePort]; ok {
		s.logger.Info("Closing proxy connection from client request",
			logger.String("device", device.Name),
			logger.String("conn_id", msg.ConnID))
		proxy.CloseConn(msg.ConnID)
	}
}

// handleDisconnect 处理连接断开
func (s *Server) handleDisconnect(conn *websocket.Conn) {
	s.mu.Lock()
	device, exists := s.conns[conn]
	if !exists {
		s.mu.Unlock()
		return
	}

	s.logger.Info("Device disconnected",
		logger.String("device", device.Name),
		logger.Int("remote_port", device.RemotePort))

	// 先从映射中删除，防止其他地方使用
	proxy := s.proxies[device.RemotePort]
	delete(s.proxies, device.RemotePort)
	s.portPool.Release(device.RemotePort)
	delete(s.devices, device.Name)
	delete(s.conns, conn)
	s.mu.Unlock()

	// 在锁外停止代理，避免死锁
	if proxy != nil {
		proxy.Stop()
	}
}

// checkHeartbeatTimeout 检查心跳超时
func (s *Server) checkHeartbeatTimeout() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var connsToClose []*websocket.Conn
			s.mu.Lock()
			now := time.Now()
			for _, device := range s.devices {
				if now.Sub(device.LastHeartbeat) > s.config.HeartbeatTimeout {
					s.logger.Warn("Device timeout",
						logger.String("device", device.Name),
						logger.Duration("timeout", s.config.HeartbeatTimeout))
					connsToClose = append(connsToClose, device.Conn)
				}
			}
			s.mu.Unlock()
			// 在锁外关闭连接，避免死锁
			for _, conn := range connsToClose {
				conn.Close()
			}
		case <-s.stopCh:
			return
		}
	}
}

// sendMessage 发送消息
func (s *Server) sendMessage(conn *websocket.Conn, msg *protocol.ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("Failed to marshal message", logger.Err(err))
		return
	}

	// 查找对应的设备获取写锁
	s.mu.RLock()
	device, exists := s.conns[conn]
	if exists {
		device.connMu.Lock()
	}
	s.mu.RUnlock()

	if exists {
		err = conn.WriteMessage(websocket.TextMessage, data)
		device.connMu.Unlock()
	} else {
		err = conn.WriteMessage(websocket.TextMessage, data)
	}

	if err != nil {
		s.logger.Error("Failed to send message", logger.Err(err))
	}
}

// sendError 发送错误消息
func (s *Server) sendError(conn *websocket.Conn, message string) {
	s.sendMessage(conn, &protocol.ServerMessage{
		Type:    protocol.TypeError,
		Message: message,
	})
}

// GetDevices 获取所有设备信息 (供Web界面使用)
func (s *Server) GetDevices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*Device, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, d)
	}
	return devices
}

// GetDevice 获取指定设备信息
func (s *Server) GetDevice(name string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[name]
}

// EnableDevice 启用设备端口
func (s *Server) EnableDevice(deviceName string, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, exists := s.devices[deviceName]
	if !exists {
		return fmt.Errorf("device not found: %s", deviceName)
	}

	if device.Enabled {
		return fmt.Errorf("device already enabled: %s", deviceName)
	}

	// 启动TCP代理
	proxy := NewProxy(device.RemotePort, device, s.config.MaxConnsPerProxy, s.logger)
	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}
	s.proxies[device.RemotePort] = proxy

	// 更新设备状态
	device.Enabled = true
	if duration > 0 {
		device.ExpireAt = time.Now().Add(duration)
	} else {
		device.ExpireAt = time.Time{} // 永久
	}

	s.logger.Info("Device enabled",
		logger.String("device", deviceName),
		logger.Int("remote_port", device.RemotePort),
		logger.Duration("duration", duration))

	return nil
}

// DisableDevice 禁用设备端口
func (s *Server) DisableDevice(deviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, exists := s.devices[deviceName]
	if !exists {
		return fmt.Errorf("device not found: %s", deviceName)
	}

	if !device.Enabled {
		return fmt.Errorf("device not enabled: %s", deviceName)
	}

	// 停止代理
	if proxy, ok := s.proxies[device.RemotePort]; ok {
		proxy.Stop()
		delete(s.proxies, device.RemotePort)
	}

	// 更新设备状态
	device.Enabled = false
	device.ExpireAt = time.Time{}

	s.logger.Info("Device disabled",
		logger.String("device", deviceName),
		logger.Int("remote_port", device.RemotePort))

	return nil
}

// checkExpiredDevices 检查过期设备
func (s *Server) checkExpiredDevices() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for _, device := range s.devices {
				if device.Enabled && !device.ExpireAt.IsZero() && now.After(device.ExpireAt) {
					s.logger.Info("Device expired, disabling",
						logger.String("device", device.Name),
						logger.Int("remote_port", device.RemotePort))

					// 停止代理
					if proxy, ok := s.proxies[device.RemotePort]; ok {
						proxy.Stop()
						delete(s.proxies, device.RemotePort)
					}

					device.Enabled = false
					device.ExpireAt = time.Time{}
				}
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}
