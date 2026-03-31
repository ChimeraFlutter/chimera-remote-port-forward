# 实现计划 - 阶段五

## 阶段五：客户端

### 目标
实现客户端的 WebSocket 连接、认证、心跳、数据转发。

### 功能描述
- 连接服务端 WebSocket
- Token 认证和设备注册
- 定时发送心跳 (30秒)
- 断线自动重连 (3秒)
- 本地 TCP 端口数据转发

### 文件列表
- `internal/client/client.go` - 客户端核心逻辑（新建）

### 实现思路

#### 1. client.go

```go
type Client struct {
    config       *config.ClientConfig
    conn         *websocket.Conn
    localConns   map[string]net.Conn   // connID -> 本地TCP连接
    mu           sync.RWMutex
    stopCh       chan struct{}
    reconnectCh  chan struct{}
}

func NewClient(cfg *config.ClientConfig) *Client
func (c *Client) Start() error
func (c *Client) Stop()
func (c *Client) connect() error
func (c *Client) register() error
func (c *Client) sendHeartbeat()
func (c *Client) handleMessage(msg *protocol.ServerMessage)
func (c *Client) handleData(connID string, data []byte)
func (c *Client) connectLocal(port int) (net.Conn, error)
```

#### 2. 核心流程

```
                    启动
                     |
                     v
              连接 WebSocket
                     |
                     v
              发送 register
                     |
              +------+------+
              |             |
              v             v
        心跳循环(30s)   消息接收循环
              |             |
              |             v
              |       处理消息:
              |       - assigned: 记录端口
              |       - heartbeat_ack: 忽略
              |       - data: 转发到本地
              |       - error: 打印错误
              |             |
              +------+------+
                     |
                     v
              连接断开?
                     |
              +------+------+
              |             |
              v             v
             是            否
              |             |
              v             |
        等待3秒重连 <--------+
```

#### 3. 数据转发流程

收到服务端 data 消息时：

```
服务端 data 消息
    |
    v
检查 localConns[connID] 是否存在
    |
    +--- 不存在 ---> 创建到 localhost:localPort 的 TCP 连接
    |                    |
    |                    v
    |               存入 localConns[connID]
    |                    |
    +--------------------+
    |
    v
写入数据到 TCP 连接
    |
    v
启动 goroutine 读取 TCP 返回数据
    |
    v
封装为 data 消息发送到服务端
```

#### 4. 自动重连

```go
func (c *Client) Start() error {
    for {
        select {
        case <-c.stopCh:
            return nil
        default:
            if err := c.connect(); err != nil {
                log.Printf("connect failed: %v, retry in 3s", err)
                time.Sleep(c.config.ReconnectInterval)
                continue
            }
            // 连接成功，开始工作
            c.run()
            // 连接断开，继续循环重连
        }
    }
}
```

#### 5. 心跳发送

```go
func (c *Client) heartbeatLoop() {
    ticker := time.NewTicker(c.config.HeartbeatInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.sendHeartbeat()
        case <-c.stopCh:
            return
        }
    }
}
