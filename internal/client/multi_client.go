package client

import (
	"fmt"
	"sync"

	"github.com/chimera/chimera-remote-port-forward/pkg/config"
	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
)

// StateCallback 状态回调函数类型
type StateCallback func(deviceName string, state int, remotePort int, message string)

// MultiClient 多端口客户端，支持同时映射多个本地端口
type MultiClient struct {
	server   string
	token    string
	clients  map[string]*Client // deviceName -> Client
	logger   *logger.Logger
	callback StateCallback
	mu       sync.RWMutex
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// 状态常量
const (
	StateDisconnected = 0
	StateConnecting   = 1
	StateConnected    = 2
	StateError        = 3
)

// NewMultiClient 创建多端口客户端
func NewMultiClient(server, token string, callback StateCallback) *MultiClient {
	return &MultiClient{
		server:   server,
		token:    token,
		clients:  make(map[string]*Client),
		callback: callback,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// SetStateCallback 设置状态回调
func (m *MultiClient) SetStateCallback(callback StateCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callback = callback
}

// SetLogger 设置日志记录器
func (m *MultiClient) SetLogger(log *logger.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = log
}

// AddPort 添加端口映射
func (m *MultiClient) AddPort(deviceName string, localIP string, localPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[deviceName]; exists {
		return fmt.Errorf("device name already exists: %s", deviceName)
	}

	if localIP == "" {
		localIP = "127.0.0.1"
	}

	cfg := &config.ClientConfig{
		Server:            m.server,
		DeviceName:        deviceName,
		LocalIP:           localIP,
		LocalPort:         localPort,
		Token:             m.token,
		HeartbeatInterval: config.DefaultClientConfig().HeartbeatInterval,
		ReconnectInterval: config.DefaultClientConfig().ReconnectInterval,
	}

	client := NewClient(cfg, m.logger)
	m.clients[deviceName] = client

	// 启动客户端
	go func() {
		m.notifyState(deviceName, StateConnecting, 0, "")

		// 包装原始 Start，监听状态
		err := client.Start()

		if err != nil {
			m.notifyState(deviceName, StateError, 0, err.Error())
		}

		m.mu.Lock()
		delete(m.clients, deviceName)
		m.mu.Unlock()
	}()

	return nil
}

// RemovePort 移除端口映射
func (m *MultiClient) RemovePort(deviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, exists := m.clients[deviceName]
	if !exists {
		return fmt.Errorf("device not found: %s", deviceName)
	}

	client.Stop()
	delete(m.clients, deviceName)
	m.notifyState(deviceName, StateDisconnected, 0, "")

	return nil
}

// GetPorts 获取所有端口映射状态
func (m *MultiClient) GetPorts() []PortInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ports := make([]PortInfo, 0, len(m.clients))
	for name, client := range m.clients {
		info := PortInfo{
			DeviceName: name,
			LocalIP:    client.Config.LocalIP,
			LocalPort:  client.Config.LocalPort,
			RemotePort: client.RemotePort,
		}
		ports = append(ports, info)
	}
	return ports
}

// PortInfo 端口信息
type PortInfo struct {
	DeviceName string
	LocalIP    string
	LocalPort  int
	RemotePort int
}

// Stop 停止所有端口映射
func (m *MultiClient) Stop() {
	close(m.stopCh)

	m.mu.Lock()
	for _, client := range m.clients {
		client.Stop()
	}
	m.clients = make(map[string]*Client)
	m.mu.Unlock()

	close(m.doneCh)
}

// Done 返回完成通道
func (m *MultiClient) Done() <-chan struct{} {
	return m.doneCh
}

// notifyState 通知状态变化
func (m *MultiClient) notifyState(deviceName string, state int, remotePort int, message string) {
	if m.callback != nil {
		m.callback(deviceName, state, remotePort, message)
	}
}

// OnConnected 当客户端连接成功时调用
func (m *MultiClient) OnConnected(deviceName string, remotePort int) {
	m.notifyState(deviceName, StateConnected, remotePort, "")
}

// OnDisconnected 当客户端断开时调用
func (m *MultiClient) OnDisconnected(deviceName string, message string) {
	m.notifyState(deviceName, StateDisconnected, 0, message)
}
