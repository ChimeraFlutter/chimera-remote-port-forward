# 实现计划 - 阶段六

## 阶段六：命令行入口

### 目标
实现统一命令行入口，根据参数区分 server/client 模式。

### 功能描述
- 解析命令行参数
- 根据 `-mode` 参数启动服务端或客户端
- 支持配置文件和命令行参数覆盖

### 文件列表
- `cmd/main.go` - 命令行入口（新建）

### 实现思路

#### 1. 命令行参数

**通用参数：**
- `-mode` - 运行模式：server 或 client（必需）
- `-config` - 配置文件路径（可选）

**服务端参数：**
- `-listen` - WebSocket 监听地址，默认 :52341
- `-web` - Web 界面地址，默认 :52342
- `-port-start` - 端口范围起始，默认 10000
- `-port-end` - 端口范围结束，默认 11000
- `-token` - 认证 Token
- `-web-password` - Web 界面密码

**客户端参数：**
- `-server` - 服务端地址
- `-device-name` - 设备名称
- `-local-port` - 本地端口
- `-token` - 认证 Token

#### 2. 参数优先级

```
命令行参数 > 配置文件 > 默认值
```

#### 3. main.go 结构

```go
func main() {
    // 定义命令行参数
    mode := flag.String("mode", "", "运行模式: server 或 client")
    configFile := flag.String("config", "", "配置文件路径")

    // 服务端参数
    listen := flag.String("listen", "", "WebSocket监听地址")
    web := flag.String("web", "", "Web界面地址")
    portStart := flag.Int("port-start", 0, "端口范围起始")
    portEnd := flag.Int("port-end", 0, "端口范围结束")
    token := flag.String("token", "", "认证Token")
    webPassword := flag.String("web-password", "", "Web界面密码")

    // 客户端参数
    server := flag.String("server", "", "服务端地址")
    deviceName := flag.String("device-name", "", "设备名称")
    localPort := flag.Int("local-port", 0, "本地端口")

    flag.Parse()

    // 根据 mode 启动对应服务
    switch *mode {
    case "server":
        startServer(...)
    case "client":
        startClient(...)
    default:
        log.Fatal("必须指定 -mode 参数: server 或 client")
    }
}
```

#### 4. 使用示例

**服务端：**
```bash
# 最简启动（使用默认值）
./chimera-remote-port-forward -mode server

# 指定参数
./chimera-remote-port-forward -mode server \
    -token "my-secret-token" \
    -web-password "admin123"

# 使用配置文件
./chimera-remote-port-forward -mode server -config server.yaml
```

**客户端：**
```bash
# 基本用法
./chimera-remote-port-forward -mode client \
    -server ws://localhost:52341/ws \
    -device-name "my-laptop" \
    -local-port 3000 \
    -token "my-secret-token"

# 使用配置文件
./chimera-remote-port-forward -mode client -config client.yaml
```

#### 5. 输出示例

**服务端启动：**
```
2024-03-31 10:00:00 [INFO] WebSocket listening on: :52341
2024-03-31 10:00:00 [INFO] Web interface on: http://localhost:52342
2024-03-31 10:00:00 [INFO] Port range: 10000-11000
```

**客户端启动：**
```
2024-03-31 10:00:05 [INFO] Connecting to ws://localhost:52341/ws...
2024-03-31 10:00:05 [INFO] Connected, registering device: my-laptop
2024-03-31 10:00:05 [INFO] Assigned remote port: 10001
2024-03-31 10:00:05 [INFO] Forwarding localhost:3000 -> server:10001
```
