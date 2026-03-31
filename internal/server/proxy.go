package server

import (
	"encoding/json"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chimera/chimera-remote-port-forward/internal/protocol"
	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
	"github.com/gorilla/websocket"
)

// 全局随机数生成器，线程安全
var globalRand = rand.New(&lockedSource{src: rand.NewSource(time.Now().UnixNano())})

// lockedSource 线程安全的随机数源
type lockedSource struct {
	mu  sync.Mutex
	src rand.Source
}

func (ls *lockedSource) Int63() int64 {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.src.Int63()
}

func (ls *lockedSource) Seed(seed int64) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.src.Seed(seed)
}

// Proxy TCP代理
type Proxy struct {
	port            int                 // 监听端口
	device          *Device             // 关联的设备
	listener        net.Listener        // TCP监听器
	conns           map[string]net.Conn // connID -> TCP连接
	mu              sync.RWMutex
	stopped         atomic.Bool // 停止标志，用于快速检查
	stopCh          chan struct{}
	logger          *logger.Logger
	maxConns        int  // 最大连接数
	connCount       int  // 当前连接数
	connCountMu     sync.Mutex
}

// NewProxy 创建代理
func NewProxy(port int, device *Device, maxConns int, logger *logger.Logger) *Proxy {
	if maxConns <= 0 {
		maxConns = 10000
	}
	return &Proxy{
		port:     port,
		device:   device,
		conns:    make(map[string]net.Conn),
		stopCh:   make(chan struct{}),
		logger:   logger,
		maxConns: maxConns,
	}
}

// Start 启动代理
func (p *Proxy) Start() error {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(p.port))
	if err != nil {
		return err
	}
	p.listener = listener

	p.logger.Info("Proxy started",
		logger.Int("port", p.port),
		logger.String("device", p.device.Name))

	go p.acceptLoop()

	return nil
}

// Stop 停止代理
func (p *Proxy) Stop() {
	if !p.stopped.CompareAndSwap(false, true) {
		return // 已经停止
	}
	close(p.stopCh)
	if p.listener != nil {
		p.listener.Close()
	}

	p.mu.Lock()
	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = make(map[string]net.Conn)
	p.mu.Unlock()
}

// acceptLoop 接受连接循环
func (p *Proxy) acceptLoop() {
	for {
		select {
		case <-p.stopCh:
			return
		default:
			conn, err := p.listener.Accept()
			if err != nil {
				select {
				case <-p.stopCh:
					return
				default:
					p.logger.Error("Accept error", logger.Err(err))
					continue
				}
			}

			// 检查连接数限制
			p.connCountMu.Lock()
			if p.connCount >= p.maxConns {
				p.connCountMu.Unlock()
				conn.Close()
				p.logger.Warn("Connection rejected: max connections reached",
					logger.Int("max_conns", p.maxConns),
					logger.Int("port", p.port))
				continue
			}
			p.connCount++
			p.connCountMu.Unlock()

			connID := generateConnID()
			p.mu.Lock()
			p.conns[connID] = conn
			p.mu.Unlock()

			p.logger.Info("New TCP connection",
				logger.String("conn_id", connID),
				logger.Int("port", p.port),
				logger.String("device", p.device.Name))

			// 通知客户端有新连接
			p.sendToClient(&protocol.ServerMessage{
				Type:   protocol.TypeConnOpen,
				ConnID: connID,
			})

			// 读取数据并转发
			go p.readLoop(connID, conn)
		}
	}
}

// readLoop 读取TCP数据并转发到客户端
func (p *Proxy) readLoop(connID string, conn net.Conn) {
	defer func() {
		// 通知客户端连接关闭
		p.sendToClient(&protocol.ServerMessage{
			Type:   protocol.TypeConnClose,
			ConnID: connID,
		})
		conn.Close()
		p.mu.Lock()
		delete(p.conns, connID)
		p.mu.Unlock()
		p.connCountMu.Lock()
		p.connCount--
		p.connCountMu.Unlock()
	}()

	buf := make([]byte, 4096)
	for {
		select {
		case <-p.stopCh:
			return
		default:
			n, err := conn.Read(buf)
			if err != nil {
				return
			}

			// 转发数据到客户端
			p.sendToClient(&protocol.ServerMessage{
				Type:   protocol.TypeData,
				ConnID: connID,
				Data:   buf[:n],
			})
		}
	}
}

// HandleFromClient 处理从客户端收到的数据
func (p *Proxy) HandleFromClient(connID string, data []byte) {
	p.mu.RLock()
	conn, exists := p.conns[connID]
	p.mu.RUnlock()

	if exists {
		// 检查是否已停止
		if p.stopped.Load() {
			return
		}
		conn.Write(data)
	}
}

// CloseConn 关闭指定连接
func (p *Proxy) CloseConn(connID string) {
	p.mu.Lock()
	if conn, exists := p.conns[connID]; exists {
		conn.Close()
		delete(p.conns, connID)
	}
	p.mu.Unlock()
}

// sendToClient 发送消息到客户端
func (p *Proxy) sendToClient(msg *protocol.ServerMessage) {
	if p.device.Conn == nil {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	p.device.connMu.Lock()
	defer p.device.connMu.Unlock()
	_ = p.device.Conn.WriteMessage(websocket.TextMessage, data)
}

// generateConnID 生成连接ID
func generateConnID() string {
	return randomString(8)
}

// randomString 生成随机字符串（线程安全）
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[globalRand.Intn(len(letters))]
	}
	return string(b)
}
