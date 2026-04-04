package client

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/chimera/chimera-remote-port-forward/internal/protocol"
	"github.com/chimera/chimera-remote-port-forward/pkg/config"
	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
	"github.com/gorilla/websocket"
)

// Client 客户端
type Client struct {
	Config     *config.ClientConfig
	conn       *websocket.Conn
	localConns map[string]net.Conn // connID -> 本地TCP连接
	RemotePort int                 // 服务端分配的远程端口（导出供外部访问）
	mu         sync.RWMutex
	connMu     sync.Mutex // 保护WebSocket写入
	stopCh     chan struct{}
	doneCh     chan struct{}
	logger     *logger.Logger
}

// NewClient 创建客户端
func NewClient(cfg *config.ClientConfig, logger *logger.Logger) *Client {
	if cfg == nil {
		cfg = config.DefaultClientConfig()
	}
	return &Client{
		Config:     cfg,
		localConns: make(map[string]net.Conn),
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
		logger:     logger,
	}
}

// Start 启动客户端
func (c *Client) Start() error {
	defer close(c.doneCh)
	reconnectCount := 0
	for {
		select {
		case <-c.stopCh:
			c.logger.Info("Start: received stop signal, exiting")
			return nil
		default:
			reconnectCount++
			c.logger.Info("Attempting connection",
				logger.Int("attempt", reconnectCount))

			if err := c.connect(); err != nil {
				c.logger.Warn("Connect failed, retrying",
					logger.Err(err),
					logger.Int("attempt", reconnectCount),
					logger.Duration("retry_interval", c.Config.ReconnectInterval))
				time.Sleep(c.Config.ReconnectInterval)
				continue
			}

			// 连接成功，注册设备
			if err := c.register(); err != nil {
				c.logger.Warn("Register failed, retrying",
					logger.Err(err),
					logger.Int("attempt", reconnectCount),
					logger.Duration("retry_interval", c.Config.ReconnectInterval))
				c.closeConn()
				time.Sleep(c.Config.ReconnectInterval)
				continue
			}

			// 运行主循环
			c.run()
		}
	}
}

// Stop 停止客户端
func (c *Client) Stop() {
	close(c.stopCh)
	c.closeConn()
	<-c.doneCh
	c.logger.Info("Client stopped")
}

// Done 返回doneCh，用于等待客户端完全停止
func (c *Client) Done() <-chan struct{} {
	return c.doneCh
}

// connect 连接服务端
func (c *Client) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(c.Config.Server, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.logger.Info("Connected to server",
		logger.String("server", c.Config.Server))
	return nil
}

// register 注册设备
func (c *Client) register() error {
	msg := &protocol.ClientMessage{
		Type:       protocol.TypeRegister,
		DeviceName: c.Config.DeviceName,
		LocalIP:    c.Config.LocalIP,
		LocalPort:  c.Config.LocalPort,
		Token:      c.Config.Token,
	}

	if err := c.sendMessage(msg); err != nil {
		return err
	}

	// 等待 assigned 消息
	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	var resp protocol.ServerMessage
	if err := json.Unmarshal(message, &resp); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	if resp.Type == protocol.TypeError {
		return fmt.Errorf("server error: %s", resp.Message)
	}

	if resp.Type != protocol.TypeAssigned {
		return fmt.Errorf("unexpected response type: %s", resp.Type)
	}

	c.RemotePort = resp.RemotePort
	c.logger.Info("Device registered",
		logger.String("device", c.Config.DeviceName),
		logger.String("local_ip", c.Config.LocalIP),
		logger.Int("local_port", c.Config.LocalPort),
		logger.Int("remote_port", c.RemotePort))

	return nil
}

// run 运行主循环
func (c *Client) run() {
	// 启动心跳
	heartbeatDone := make(chan struct{})
	go c.heartbeatLoop(heartbeatDone)

	// 消息处理循环
	defer func() {
		c.logger.Info("run: exiting main loop, starting cleanup")
		close(heartbeatDone)
		c.cleanup()
		c.logger.Info("run: cleanup completed, will attempt reconnect")
	}()

	for {
		select {
		case <-c.stopCh:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				c.logger.Warn("Connection lost", logger.Err(err))
				return
			}

			var msg protocol.ServerMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				c.logger.Error("Invalid message", logger.Err(err))
				continue
			}

			c.handleMessage(&msg)
		}
	}
}

// heartbeatLoop 心跳循环
func (c *Client) heartbeatLoop(done <-chan struct{}) {
	ticker := time.NewTicker(c.Config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.logger.Warn("Send heartbeat failed, closing connection to trigger reconnect", logger.Err(err))
				c.closeConn() // 主动关闭连接，触发重连
				return
			}
		case <-done:
			c.logger.Info("heartbeatLoop: received done signal, exiting")
			return
		case <-c.stopCh:
			c.logger.Info("heartbeatLoop: received stop signal, exiting")
			return
		}
	}
}

// sendHeartbeat 发送心跳
func (c *Client) sendHeartbeat() error {
	return c.sendMessage(&protocol.ClientMessage{
		Type: protocol.TypeHeartbeat,
	})
}

// handleMessage 处理服务端消息
func (c *Client) handleMessage(msg *protocol.ServerMessage) {
	switch msg.Type {
	case protocol.TypeHeartbeatAck:
		// 心跳响应，忽略
	case protocol.TypeConnOpen:
		c.logger.Info("Received conn_open from server",
			logger.String("conn_id", msg.ConnID))
		c.handleConnOpen(msg.ConnID)
	case protocol.TypeData:
		c.logger.Info("Received data from server",
			logger.String("conn_id", msg.ConnID),
			logger.Int("bytes", len(msg.Data)))
		c.handleData(msg.ConnID, msg.Data)
	case protocol.TypeConnClose:
		c.logger.Info("Received conn_close from server",
			logger.String("conn_id", msg.ConnID))
		c.handleConnClose(msg.ConnID)
	case protocol.TypeError:
		c.logger.Error("Server error", logger.String("message", msg.Message))
	default:
		c.logger.Warn("Unknown message type from server",
			logger.String("type", msg.Type))
	}
}

// handleConnOpen 处理新连接建立
func (c *Client) handleConnOpen(connID string) {
	localAddr := fmt.Sprintf("%s:%d", c.Config.LocalIP, c.Config.LocalPort)
	conn, err := net.Dial("tcp", localAddr)
	if err != nil {
		c.logger.Error("Connect to local port failed",
			logger.Err(err),
			logger.String("local_addr", localAddr),
			logger.String("conn_id", connID))
		// 通知服务端连接失败
		c.sendConnClose(connID)
		return
	}

	c.mu.Lock()
	c.localConns[connID] = conn
	c.mu.Unlock()

	go c.readFromLocal(connID, conn)

	c.logger.Info("Local connection established",
		logger.String("conn_id", connID))
}

// handleConnClose 处理连接关闭
func (c *Client) handleConnClose(connID string) {
	c.closeLocalConn(connID)
	c.logger.Info("Local connection closed",
		logger.String("conn_id", connID))
}

// handleData 处理数据转发
func (c *Client) handleData(connID string, data []byte) {
	c.mu.RLock()
	localConn, exists := c.localConns[connID]
	c.mu.RUnlock()

	if !exists {
		c.logger.Warn("Received data for unknown connection",
			logger.String("conn_id", connID))
		// 通知服务端连接不存在
		c.sendConnClose(connID)
		return
	}

	// 写入数据到本地连接
	n, err := localConn.Write(data)
	if err != nil {
		c.logger.Error("Write to local failed",
			logger.Err(err),
			logger.String("conn_id", connID))
		c.closeLocalConn(connID)
	} else {
		c.logger.Info("Wrote data to local",
			logger.String("conn_id", connID),
			logger.Int("bytes", n))
	}
}

// readFromLocal 从本地连接读取数据并发送到服务端
func (c *Client) readFromLocal(connID string, conn net.Conn) {
	buf := make([]byte, 32*1024)
	defer func() {
		c.closeLocalConn(connID)
		// 通知服务端连接关闭
		c.sendConnClose(connID)
	}()

	for {
		n, err := conn.Read(buf)
		if err != nil {
			c.logger.Info("Local connection read ended",
				logger.String("conn_id", connID),
				logger.Err(err))
			return
		}

		c.logger.Info("Read from local, sending to server",
			logger.String("conn_id", connID),
			logger.Int("bytes", n))

		if err := c.sendMessage(&protocol.ClientMessage{
			Type:   protocol.TypeData,
			ConnID: connID,
			Data:   buf[:n],
		}); err != nil {
			c.logger.Error("Send data to server failed",
				logger.Err(err),
				logger.String("conn_id", connID))
			return
		}
	}
}

// closeLocalConn 关闭本地连接
func (c *Client) closeLocalConn(connID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.localConns[connID]; exists {
		conn.Close()
		delete(c.localConns, connID)
	}
}

// cleanup 清理资源
func (c *Client) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭所有本地连接
	for _, conn := range c.localConns {
		conn.Close()
	}
	c.localConns = make(map[string]net.Conn)

	// 使用 closeConnUnsafe，因为我们已经持有锁
	c.closeConnUnsafe()
}

// closeConn 关闭WebSocket连接（必须在持有 c.mu 锁的情况下调用，或者不在 cleanup 中调用）
func (c *Client) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// closeConnUnsafe 关闭WebSocket连接（调用者必须持有 c.mu 锁）
func (c *Client) closeConnUnsafe() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// sendMessage 发送消息
func (c *Client) sendMessage(msg *protocol.ClientMessage) error {
	c.connMu.Lock()
	c.mu.RLock()
	if c.conn == nil {
		c.mu.RUnlock()
		c.connMu.Unlock()
		return fmt.Errorf("connection not established")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		c.mu.RUnlock()
		c.connMu.Unlock()
		c.logger.Error("sendMessage: marshal failed", logger.Err(err))
		return err
	}
	conn := c.conn
	c.mu.RUnlock()

	err = conn.WriteMessage(websocket.TextMessage, data)
	c.connMu.Unlock()
	if err != nil {
		c.logger.Error("sendMessage: write failed",
			logger.Err(err),
			logger.String("type", msg.Type),
			logger.String("conn_id", msg.ConnID))
	} else {
		c.logger.Debug("sendMessage: success",
			logger.String("type", msg.Type),
			logger.String("conn_id", msg.ConnID),
			logger.Int("bytes", len(data)))
	}
	return err
}

// sendConnClose 发送连接关闭消息给服务端
func (c *Client) sendConnClose(connID string) {
	err := c.sendMessage(&protocol.ClientMessage{
		Type:   protocol.TypeConnClose,
		ConnID: connID,
	})
	if err != nil {
		c.logger.Warn("Failed to send conn_close to server",
			logger.Err(err),
			logger.String("conn_id", connID))
	}
}
