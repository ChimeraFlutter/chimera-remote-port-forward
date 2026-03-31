# 日志使用指南

## 概述

本项目使用结构化日志系统，支持：
- 按小时自动分割日志文件
- 自动清理过期日志（默认7天）
- 文本格式，便于AI分析和问题定位

## 日志路径

默认路径：`C:/logs/chimera-remote-port-forward/chimera-monitor-backend/`

文件命名格式：
```
{日志目录}/{服务名}/{日期}/raw-{日期}-{小时}.log
```

示例：
```
C:/logs/chimera-remote-port-forward/chimera-monitor-backend/2026-03-22/raw-2026-03-22-20.log
C:/logs/chimera-remote-port-forward/chimera-monitor-backend/2026-03-22/raw-2026-03-22-21.log
```

## 日志格式

每条日志格式如下：
```
{时间戳} {级别} {消息} {字段...}
```

示例：
```
2026-03-22T20:15:30.123+08:00 INFO  Device registered device=my-device local_port=8080 remote_port=10001
2026-03-22T20:15:31.456+08:00 ERROR Connect failed error="connection refused" device=my-device
```

### 字段说明

| 字段 | 说明 | 示例 |
|------|------|------|
| device | 设备名称 | my-device |
| conn_id | 连接ID | aBcD1234 |
| local_port | 本地端口 | 8080 |
| remote_port | 远程端口 | 10001 |
| client_addr | 客户端地址 | 192.168.1.100:54321 |
| error | 错误信息 | connection refused |

## 配置方式

### 命令行参数

```bash
# 服务端
./chimera-remote-port-forward -mode server \
    -log-dir "C:/logs/chimera-remote-port-forward" \
    -log-max-age 7

# 客户端
./chimera-remote-port-forward -mode client \
    -server ws://localhost:52341/ws \
    -device-name my-device \
    -local-port 8080 \
    -log-dir "C:/logs/chimera-remote-port-forward" \
    -log-max-age 7
```

### 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| -log-dir | C:/logs/chimera-remote-port-forward | 日志根目录 |
| -log-max-age | 7 | 日志保留天数 |

## 日志级别

| 级别 | 说明 |
|------|------|
| DEBUG | 调试信息 |
| INFO | 正常运行信息 |
| WARN | 警告信息 |
| ERROR | 错误信息 |

## 常见日志示例

### 设备注册成功
```
2026-03-22T20:15:30.123+08:00 INFO  Device registered device=my-device local_port=8080 remote_port=10001
```

### 设备断开连接
```
2026-03-22T20:20:30.456+08:00 INFO  Device disconnected device=my-device remote_port=10001
```

### 心跳超时
```
2026-03-22T21:00:00.000+08:00 WARN  Device timeout device=my-device timeout=90000
```

### 连接失败
```
2026-03-22T20:15:31.456+08:00 ERROR Connect failed error="connection refused" device=my-device
```

### 新TCP连接
```
2026-03-22T20:16:00.789+08:00 INFO  New TCP connection conn_id=aBcD1234 port=10001 device=my-device
```

## AI分析建议

### 搜索关键词

按设备搜索：
```bash
grep "device=my-device" raw-2026-03-22-20.log
```

按端口搜索：
```bash
grep "remote_port=10001" raw-2026-03-22-20.log
```

按级别搜索：
```bash
grep "ERROR" raw-2026-03-22-20.log
```

按连接ID追踪：
```bash
grep "conn_id=aBcD1234" raw-2026-03-22-20.log
```

### 常见问题定位

1. **设备无法连接**
   - 搜索 `ERROR` 和 `Connect failed`
   - 检查 `error=` 字段

2. **设备频繁断开**
   - 搜索 `Device disconnected` 和 `Device timeout`
   - 检查时间间隔是否接近心跳超时（默认90秒）

3. **端口分配失败**
   - 搜索 `failed to start proxy`
   - 检查端口范围是否耗尽

## 日志轮转

- **按小时轮转**：每小时自动创建新文件
- **按日期分目录**：每天的日志放在独立目录
- **自动清理**：超过保留天数的日志目录会被自动删除

## 日志目录结构

```
C:/logs/chimera-remote-port-forward/
└── chimera-monitor-backend/
    ├── 2026-03-20/
    │   ├── raw-2026-03-20-00.log
    │   ├── raw-2026-03-20-01.log
    │   └── ...
    ├── 2026-03-21/
    │   ├── raw-2026-03-21-00.log
    │   └── ...
    └── 2026-03-22/
        ├── raw-2026-03-22-00.log
        ├── raw-2026-03-22-01.log
        └── ...
```
