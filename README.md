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

## 功能特性

- **远程端口转发**：将内网服务暴露到公网
- **Web 管理界面**：可视化管理所有设备
- **手动启用/禁用**：在 Web 界面控制端口开关
- **有效期设置**：支持 1h/6h/12h/24h/永久，过期自动关闭
- **实时倒计时**：显示剩余时间，到期前警告
- **多端口映射**：DLL 版本支持同时映射多个端口
- **跨平台 DLL**：支持 Flutter/其他语言通过 FFI 调用

## 编译

```bash
# 编译可执行文件
go build -o chimera-remote-port-forward ./cmd

# 编译 Windows DLL (需要交叉编译)
GOOS=windows GOARCH=amd64 go build -buildmode=c-shared -o chimera.dll ./cmd/dll
GOOS=windows GOARCH=arm64 go build -buildmode=c-shared -o chimera_arm64.dll ./cmd/dll

# 编译 Linux so
GOOS=linux GOARCH=amd64 go build -buildmode=c-shared -o libchimera.so ./cmd/dll

# 编译 macOS dylib
go build -buildmode=c-shared -o libchimera.dylib ./cmd/dll
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

# 使用配置文件
./chimera-remote-port-forward -mode server -config server.yaml
```

### 服务端参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mode` | (必需) | 运行模式，必须为 `server` 或 `client` |
| `-config` | (空) | 配置文件路径，支持 YAML 格式 |
| `-listen` | `:52341` | WebSocket 监听地址 |
| `-web` | `:52342` | Web 管理界面地址 |
| `-port-start` | `10000` | 端口池起始端口 |
| `-port-end` | `11000` | 端口池结束端口 |
| `-token` | (空) | 客户端认证 Token，为空则不验证 |
| `-web-password` | `admin` | Web 管理界面密码 |
| `-log-dir` | (见说明) | 日志目录，默认跨平台适配 |
| `-log-max-age` | `7` | 日志保留天数 |

### 服务端配置文件示例 (server.yaml)

```yaml
listen: ":52341"
web: ":52342"
port_start: 10000
port_end: 11000
heartbeat_timeout: 90s
auth_token: "your-secret-token"
web_password: "admin123"
log_dir: "/var/log/chimera-remote-port-forward"
log_max_age: 7
max_devices: 1000
max_conns_per_proxy: 10000
```

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

# 使用配置文件
./chimera-remote-port-forward -mode client -config client.yaml
```

### 客户端参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-mode` | (必需) | 运行模式，必须为 `server` 或 `client` |
| `-config` | (空) | 配置文件路径，支持 YAML 格式 |
| `-server` | (必需) | 服务端 WebSocket 地址 |
| `-device-name` | (必需) | 设备名称，需全局唯一 |
| `-local-port` | (必需) | 要转发的本地端口 |
| `-token` | (空) | 认证 Token，需与服务端一致 |
| `-log-dir` | (见说明) | 日志目录，默认跨平台适配 |
| `-log-max-age` | `7` | 日志保留天数 |

### 客户端配置文件示例 (client.yaml)

```yaml
server: "ws://YOUR_SERVER:52341/ws"
device_name: "my-device"
local_port: 8080
token: "your-secret-token"
heartbeat_interval: 30s
reconnect_interval: 3s
log_dir: "/var/log/chimera-remote-port-forward"
log_max_age: 7
```

### 配置优先级

命令行参数优先级高于配置文件。如果同时指定配置文件和命令行参数，命令行参数会覆盖配置文件中的对应项。

## Web 管理界面

### 设备管理

1. 访问 `http://SERVER_IP:52342`
2. 使用密码登录（默认 `admin`）
3. 查看所有已连接的设备

### 端口启用/禁用

设备注册后，默认处于**禁用**状态，需要在 Web 界面手动启用：

1. 点击设备行的 **Enable** 按钮
2. 选择有效期：
   - 1 Hour
   - 6 Hours
   - 12 Hours
   - **24 Hours (Default)**
   - Permanent (No Expiry)
3. 确认后端口开始监听
4. 界面显示实时倒计时
5. 随时可点击 **Disable** 关闭端口

### 自动过期

- 设置有效期的端口会在到期后自动关闭
- 剩余时间少于 1 小时时显示红色警告
- 后台每分钟检查一次过期状态

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

3. **在 Web 界面启用端口**
   - 访问 `http://公网服务器IP:52342`
   - 登录后点击 Enable 按钮
   - 选择有效期并确认

4. **访问服务**
   - 服务端会分配一个端口（如 10000）
   - 访问 `http://公网服务器IP:10000` 即可访问内网的 Web 服务

## DLL 接口 (Flutter/其他语言)

编译 DLL 后，可通过 FFI 调用。以下是接口说明：

### 导出函数

```c
// 初始化客户端
// 返回: 0 成功, -1 失败
int Initialize(const char* server, const char* token);

// 设置状态回调
// callback: void callback(const char* deviceName, int state, int remotePort, const char* message)
// state: 0=断开, 1=连接中, 2=已连接, 3=错误
int SetStateCallback(void* callback);

// 添加端口映射
// deviceName: 设备名称（需唯一）
// localPort: 本地端口
// 返回: 0 成功, -1 失败
int AddPort(const char* deviceName, int localPort);

// 移除端口映射
int RemovePort(const char* deviceName);

// 获取端口映射数量
int GetPortCount();

// 获取端口信息
// index: 索引 (0 ~ GetPortCount()-1)
// deviceNameBuf: 设备名输出缓冲区
// bufSize: 缓冲区大小
// localPort: 本地端口输出
// remotePort: 远程端口输出
int GetPortInfo(int index, char* deviceNameBuf, int bufSize, int* localPort, int* remotePort);

// 停止所有连接
int Stop();

// 获取版本号
const char* GetVersion();
```

### Flutter 调用示例

```dart
import 'dart:ffi';
import 'package:ffi/ffi.dart';

typedef InitializeFunc = Int32 Function(Pointer<Utf8> server, Pointer<Utf8> token);
typedef AddPortFunc = Int32 Function(Pointer<Utf8> deviceName, Int32 localPort);
typedef RemovePortFunc = Int32 Function(Pointer<Utf8> deviceName);
typedef GetPortCountFunc = Int32 Function();
typedef StopFunc = Int32 Function();

class ChimeraClient {
  final DynamicLibrary _lib;
  
  ChimeraClient(String path) : _lib = DynamicLibrary.open(path);
  
  int initialize(String server, String token) {
    final func = _lib.lookupFunction<InitializeFunc, InitializeFunc>('Initialize');
    return func(server.toNativeUtf8(), token.toNativeUtf8());
  }
  
  int addPort(String deviceName, int localPort) {
    final func = _lib.lookupFunction<AddPortFunc, AddPortFunc>('AddPort');
    return func(deviceName.toNativeUtf8(), localPort);
  }
  
  int removePort(String deviceName) {
    final func = _lib.lookupFunction<RemovePortFunc, RemovePortFunc>('RemovePort');
    return func(deviceName.toNativeUtf8());
  }
  
  int getPortCount() {
    final func = _lib.lookupFunction<GetPortCountFunc, GetPortCountFunc>('GetPortCount');
    return func();
  }
  
  int stop() {
    final func = _lib.lookupFunction<StopFunc, StopFunc>('Stop');
    return func();
  }
}

// 使用示例
void main() {
  final client = ChimeraClient('chimera.dll');
  
  // 初始化
  client.initialize('ws://server:52341/ws', 'your-token');
  
  // 添加多个端口映射
  client.addPort('web-server', 8080);
  client.addPort('api-server', 3000);
  client.addPort('db-server', 5432);
  
  // 获取映射数量
  print('Active ports: ${client.getPortCount()}');
  
  // 移除单个映射
  client.removePort('api-server');
  
  // 停止所有
  client.stop();
}
```

## 注意事项

- 客户端设备名称必须唯一，重复注册会被拒绝
- 服务端默认端口池为 10000-11000，最多支持 1000 个设备
- **生产环境建议**：
  - 启用 `-token` 认证
  - 修改默认 Web 密码
  - 设置合理的端口有效期，避免长期暴露
