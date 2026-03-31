# 实现计划 - 阶段二

## 阶段二：服务端核心

### 目标
实现服务端的 WebSocket 处理、端口池管理、设备注册、心跳检测。

### 功能描述
- 端口池管理：分配和回收端口
- WebSocket 连接处理
- 客户端认证和设备注册
- 心跳超时检测

### 文件列表
- `internal/server/port_pool.go` - 端口池管理（新建）
- `internal/server/server.go` - 服务端核心逻辑（新建）

### 实现思路

#### 1. port_pool.go

```go
type PortPool struct {
    mu       sync.Mutex
    start    int
    end      int
    used     map[int]bool          // 已分配端口
    bindings map[int]*Connection   // 端口 -> 连接映射
}

func NewPortPool(start, end int) *PortPool
func (p *PortPool) Allocate(conn *Connection) (int, error)  // 分配端口
func (p *PortPool) Release(port int)                         // 释放端口
func (p *PortPool) GetBinding(port int) *Connection          // 获取绑定
```

#### 2. server.go

```go
type Server struct {
    config     *config.ServerConfig
    portPool   *PortPool
    devices    map[string]*Device       // 设备名 -> 设备信息
    conns      map[*websocket.Conn]*Device
    mu         sync.RWMutex
    upgrader   websocket.Upgrader
}

type Device struct {
    Name       string
    LocalPort  int
    RemotePort int
    Conn       *websocket.Conn
    LastHeartbeat time.Time
}

func NewServer(cfg *config.ServerConfig) *Server
func (s *Server) Start() error
func (s *Server) handleWebSocket(conn *websocket.Conn)
func (s *Server) handleRegister(conn *websocket.Conn, msg *protocol.ClientMessage)
func (s *Server) handleHeartbeat(conn *websocket.Conn)
func (s *Server) checkHeartbeatTimeout()    // 后台 goroutine 检查超时
```

**流程：**
1. 启动 HTTP 服务，监听 52341 端口
2. `/ws` 路径升级为 WebSocket
3. 收到 register 消息：
   - 验证 Token
   - 分配端口
   - 记录设备信息
   - 返回 assigned 消息
4. 收到 heartbeat 消息：
   - 更新 LastHeartbeat 时间
   - 返回 heartbeat_ack
5. 后台 goroutine 每隔 10 秒检查所有设备：
   - 超过 HeartbeatTimeout 未收到心跳，断开连接，释放端口
