# 实现计划 - 阶段三

## 阶段三：服务端 TCP 代理

### 目标
实现服务端到客户端的 TCP 数据转发。

### 功能描述
- 监听分配的端口
- 接收外部 TCP 连接
- 通过 WebSocket 转发数据到客户端

### 文件列表
- `internal/server/proxy.go` - TCP 代理转发（新建）
- `internal/server/server.go` - 添加代理启动逻辑（修改）

### 实现思路

#### 1. proxy.go

```go
type Proxy struct {
    port       int
    device     *Device
    listener   net.Listener
    conns      map[string]net.Conn    // connectionID -> TCP连接
    mu         sync.RWMutex
}

type ProxyMessage struct {
    ConnID string  // 连接标识
    Data   []byte  // 数据
}

func NewProxy(port int, device *Device) *Proxy
func (p *Proxy) Start() error
func (p *Proxy) Stop()
func (p *Proxy) HandleFromClient(connID string, data []byte)  // 从客户端收到数据，写入TCP
```

**流程：**
1. 为每个分配的端口启动 TCP 监听
2. 外部请求连接时，生成唯一 connID
3. 通过 WebSocket 发送 data 消息给客户端，携带 connID
4. 收到客户端 data 消息时，根据 connID 找到对应 TCP 连接，写入数据
5. TCP 连接关闭时，通知客户端关闭对应连接

#### 2. server.go 修改

- 添加 `proxies map[int]*Proxy` 字段
- 注册成功后启动对应端口的 Proxy
- 设备断开时停止 Proxy
- 处理 data 消息时转发给 Proxy

### 数据流详解

```
外部客户端                    服务端                      内部客户端
    |                          |                           |
    |--- TCP连接 10001 ------->|                           |
    |                          |--- WS data(connID) ------>|
    |                          |                           |--- TCP连接 localhost:3000
    |                          |<-- WS data(connID) -------|
    |<-- TCP数据 --------------|                           |
    |                          |                           |
```

### 消息格式扩展

为支持多连接，data 消息需要携带连接 ID：

```go
type ClientMessage struct {
    Type     string `json:"type"`
    ConnID   string `json:"conn_id"`   // 新增：连接标识
    Data     []byte `json:"data"`
    // ...
}

type ServerMessage struct {
    Type     string `json:"type"`
    ConnID   string `json:"conn_id"`   // 新增：连接标识
    Data     []byte `json:"data"`
    // ...
}
```
