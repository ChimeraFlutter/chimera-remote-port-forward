# 实现计划 - 阶段四

## 阶段四：Web 管理界面

### 目标
提供 Web 界面查看所有端口映射状态。

### 功能描述
- HTTP 服务 (端口 52342)
- 密码保护登录
- 显示所有设备映射信息

### 文件列表
- `internal/server/web.go` - Web 界面服务（新建）
- `internal/server/server.go` - 添加 Web 服务启动逻辑（修改）

### 实现思路

#### 1. web.go

```go
type WebServer struct {
    server   *Server
    sessions map[string]time.Time  // session -> 过期时间
    mu       sync.RWMutex
}

func NewWebServer(server *Server) *WebServer
func (w *WebServer) Start(addr string) error

// HTTP 处理函数
func (w *WebServer) handleIndex(w http.ResponseWriter, r *http.Request)
func (w *WebServer) handleLogin(w http.ResponseWriter, r *http.Request)
func (w *WebServer) handleDevices(w http.ResponseWriter, r *http.Request)
```

#### 2. API 接口

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | / | 首页，显示设备列表 | 需要 |
| POST | /api/login | 登录验证，返回 session token | 不需要 |
| GET | /api/devices | 获取设备列表 JSON | 需要 |

#### 3. 登录流程

```
客户端                              服务端
   |                                  |
   |--- POST /api/login ------------->|
   |     { "password": "admin123" }   |
   |                                  | 验证密码
   |<-- { "token": "session-xxx" } ---|
   |                                  |
   |--- GET /api/devices ------------>|
   |     Header: Authorization: Bearer session-xxx
   |<-- 设备列表 JSON -----------------|
   |                                  |
```

#### 4. 页面展示信息

| 字段 | 说明 |
|------|------|
| Device Name | 设备名称 |
| Local Port | 本地端口 |
| Remote Port | 远程端口 |
| Status | 在线状态 |
| Last Heartbeat | 最后心跳时间 |
| Connections | 当前连接数 |

#### 5. server.go 修改

- `Start()` 方法中启动 Web 服务 goroutine
- 添加 `GetDevices()` 方法供 Web 调用
