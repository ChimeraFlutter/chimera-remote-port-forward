# 实现计划

本文档分阶段描述实现步骤。

---

## 阶段一：通用部分

### 目标
实现服务端和客户端共用的基础代码，包括消息协议和配置管理。

### 功能描述
- 定义 WebSocket 通信的消息结构体
- 定义服务端和客户端的配置结构体

### 文件列表
- `go.mod` - Go 模块定义（新建）
- `internal/protocol/message.go` - 消息协议定义（新建）
- `pkg/config/config.go` - 配置结构体（新建）

### 实现思路
1. **go.mod**：
   - 声明模块名 `module github.com/chimera/chimera-remote-port-forward`
   - 添加依赖：`github.com/gorilla/websocket`

2. **message.go**：
   - 定义 `ClientMessage` 结构体：
     - `Type string` - 消息类型：register, heartbeat, data
     - `DeviceName string` - 设备名称
     - `LocalPort int` - 本地端口
     - `Token string` - 认证 Token
     - `Data []byte` - 转发数据
   - 定义 `ServerMessage` 结构体：
     - `Type string` - 消息类型：assigned, heartbeat_ack, data, error
     - `RemotePort int` - 分配的远程端口
     - `Data []byte` - 转发数据
     - `Message string` - 错误信息

3. **config.go**：
   - 定义 `ServerConfig` 结构体：
     - `Listen string` - WebSocket 监听地址 ":52341"
     - `Web string` - Web 界面地址 ":52342"
     - `PortStart int` - 端口范围起始 10000
     - `PortEnd int` - 端口范围结束 11000
     - `HeartbeatTimeout time.Duration` - 心跳超时 90s
     - `AuthToken string` - 认证 Token
     - `WebPassword string` - Web 密码
   - 定义 `ClientConfig` 结构体：
     - `Server string` - 服务端地址
     - `DeviceName string` - 设备名称
     - `LocalPort int` - 本地端口
     - `Token string` - 认证 Token
     - `HeartbeatInterval time.Duration` - 心跳间隔 30s
     - `ReconnectInterval time.Duration` - 重连间隔 3s
