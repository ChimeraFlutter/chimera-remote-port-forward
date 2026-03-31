# 架构设计

## 项目结构

```
chimera-remote-port-forward/
├── cmd/
│   └── main.go              # 统一入口，根据 -mode 参数区分
├── internal/
│   ├── server/
│   │   ├── server.go        # 服务端核心逻辑
│   │   ├── port_pool.go     # 端口池管理
│   │   └── web.go           # Web界面
│   ├── client/
│   │   └── client.go        # 客户端核心逻辑
│   └── protocol/
│       └── message.go       # WebSocket消息协议
├── pkg/config/
│   └── config.go            # 配置管理
├── docs/
│   ├── design.md            # 设计文档
│   ├── architecture.md      # 架构文档
│   └── protocol.md          # 协议文档
├── go.mod
└── README.md
```

## 消息协议 (WebSocket JSON)

### 客户端 -> 服务端

```go
type ClientMessage struct {
    Type       string `json:"type"`        // register, heartbeat, data
    DeviceName string `json:"device_name"` // 设备名称
    LocalPort  int    `json:"local_port"`  // 本地端口
    Token      string `json:"token"`       // 认证Token (仅register时使用)
    Data       []byte `json:"data"`        // 转发数据
}
```

### 服务端 -> 客户端

```go
type ServerMessage struct {
    Type       string `json:"type"`        // assigned, heartbeat_ack, data, error
    RemotePort int    `json:"remote_port"` // 分配的远程端口
    Data       []byte `json:"data"`        // 转发数据
    Message    string `json:"message"`     // 错误信息
}
```

## 数据流

```
外部请求 -> 服务端端口(10000+) -> WebSocket -> 客户端 -> 本地端口
```

## 核心模块

### 1. 服务端 (server)

- WebSocket 服务监听 (端口 52341)
- 端口池管理 (分配/释放)
- 设备注册与映射表
- 心跳检测 (超时断开)
- TCP 端口转发代理
- Web 管理界面 (端口 52342)

### 2. 客户端 (client)

- WebSocket 连接管理
- Token 认证
- 设备注册
- 心跳发送 (间隔30秒)
- 断线自动重连 (间隔3秒)
- 本地 TCP 连接转发

## 配置

### 服务端配置 (server.yaml)

```yaml
listen: ":52341"              # WebSocket监听地址
web: ":52342"                 # Web界面地址
port_range:
  start: 10000
  end: 11000
heartbeat_timeout: 90s        # 心跳超时
auth_token: "your-secret-token"  # 客户端连接Token验证
web_password: "admin123"      # Web界面密码保护
```

### 客户端配置 (client.yaml)

```yaml
server: "ws://localhost:52341/ws"
device_name: "my-device"
local_port: 3000              # 要映射的本地端口
token: "your-secret-token"    # 连接Token
heartbeat_interval: 30s       # 心跳间隔
reconnect_interval: 3s        # 重连间隔
```
