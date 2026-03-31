# Chimera Remote Port Forward

远程端口转发服务，支持将内网设备的服务暴露到公网。

## 架构

```
┌─────────────┐                    ┌─────────────┐
│   Client    │◄────WebSocket────►│   Server    │
│  (内网设备)  │                    │  (公网服务)  │
└──────┬──────┘                    └──────┬──────┘
       │                                  │
       ▼                                  ▼
  本地服务端口                         远程端口
  (如 localhost:8080)               (如 :10000)
```

## 编译

```bash
go build -o chimera-remote-port-forward ./cmd
```

## 服务端启动

```bash
# 基本启动
./chimera-remote-port-forward -mode server

# 完整参数
./chimera-remote-port-forward -mode server \
  -listen :52341 \
  -web :52342 \
  -port-start 10000 \
  -port-end 11000 \
  -token YOUR_SECRET_TOKEN \
  -web-password admin123 \
  -log-dir /var/log/chimera \
  -log-max-age 7
```

### 服务端参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mode` | (必需) | 运行模式，必须为 `server` 或 `client` |
| `-listen` | `:52341` | WebSocket 监听地址 |
| `-web` | `:52342` | Web 管理界面地址 |
| `-port-start` | `10000` | 端口池起始端口 |
| `-port-end` | `11000` | 端口池结束端口 |
| `-token` | (空) | 客户端认证 Token，为空则不验证 |
| `-web-password` | `admin` | Web 管理界面密码 |
| `-log-dir` | `C:/logs/chimera-remote-port-forward` | 日志目录 |
| `-log-max-age` | `7` | 日志保留天数 |

## 客户端启动

```bash
# 基本启动
./chimera-remote-port-forward -mode client \
  -server ws://YOUR_SERVER:52341/ws \
  -device-name my-device \
  -local-port 8080

# 带认证
./chimera-remote-port-forward -mode client \
  -server ws://YOUR_SERVER:52341/ws \
  -device-name my-device \
  -local-port 8080 \
  -token YOUR_SECRET_TOKEN
```

### 客户端参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mode` | (必需) | 运行模式，必须为 `server` 或 `client` |
| `-server` | (必需) | 服务端 WebSocket 地址 |
| `-device-name` | (必需) | 设备名称，需全局唯一 |
| `-local-port` | (必需) | 要转发的本地端口 |
| `-token` | (空) | 认证 Token，需与服务端一致 |
| `-log-dir` | `C:/logs/chimera-remote-port-forward` | 日志目录 |
| `-log-max-age` | `7` | 日志保留天数 |

## 使用示例

假设你有一台内网机器运行着 Web 服务 (端口 8080)，希望从公网访问：

1. **在公网服务器上启动服务端**
   ```bash
   ./chimera-remote-port-forward -mode server -token mytoken123
   ```

2. **在内网机器上启动客户端**
   ```bash
   ./chimera-remote-port-forward -mode client \
     -server ws://公网服务器IP:52341/ws \
     -device-name my-web-server \
     -local-port 8080 \
     -token mytoken123
   ```

3. **访问服务**
   - 服务端会自动分配一个端口（如 10000）
   - 访问 `http://公网服务器IP:10000` 即可访问内网的 Web 服务

4. **Web 管理界面**
   - 访问 `http://公网服务器IP:52342`
   - 使用密码登录（默认 `admin`）
   - 查看所有已连接的设备

## 注意事项

- 客户端设备名称必须唯一，重复注册会被拒绝
- 服务端默认端口池为 10000-11000，最多支持 1000 个设备
- 生产环境建议启用 `-token` 认证并修改默认 Web 密码
