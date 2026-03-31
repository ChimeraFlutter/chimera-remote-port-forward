# 协议文档

## WebSocket 消息类型

所有消息均为 JSON 格式。

### 客户端消息类型

#### 1. 注册消息 (register)

客户端连接后首先发送，用于认证和申请端口。

```json
{
    "type": "register",
    "device_name": "my-laptop",
    "local_port": 3000,
    "token": "your-secret-token"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| type | string | 是 | 固定值 "register" |
| device_name | string | 是 | 设备名称，用于标识 |
| local_port | int | 是 | 要映射的本地端口 |
| token | string | 是 | 认证令牌 |

#### 2. 心跳消息 (heartbeat)

客户端定期发送，保持连接。

```json
{
    "type": "heartbeat"
}
```

#### 3. 数据消息 (data)

转发 TCP 数据。

```json
{
    "type": "data",
    "data": "base64-encoded-data"
}
```

### 服务端消息类型

#### 1. 端口分配消息 (assigned)

注册成功后返回分配的端口。

```json
{
    "type": "assigned",
    "remote_port": 10001
}
```

#### 2. 心跳响应 (heartbeat_ack)

响应心跳消息。

```json
{
    "type": "heartbeat_ack"
}
```

#### 3. 数据消息 (data)

转发 TCP 数据。

```json
{
    "type": "data",
    "data": "base64-encoded-data"
}
```

#### 4. 错误消息 (error)

发生错误时返回。

```json
{
    "type": "error",
    "message": "authentication failed"
}
```

## 连接生命周期

```
客户端                              服务端
   |                                  |
   |-------- register -------------->|
   |                                  | 验证 Token
   |                                  | 分配端口
   |<------- assigned ----------------|
   |                                  |
   |-------- heartbeat -------------->|  (每30秒)
   |<------- heartbeat_ack -----------|
   |                                  |
   |-------- data ------------------->|  TCP数据转发
   |<------- data --------------------|
   |                                  |
   |-------- heartbeat -------------->|  (超时则断开)
   |                                  |
   |        连接断开                   |
   |                                  | 释放端口
   |                                  |
   |-------- 重连 (每3秒) ------------>|
   |                                  |
```

## 错误码

| 错误信息 | 说明 |
|----------|------|
| authentication failed | Token 验证失败 |
| port pool exhausted | 端口池耗尽 |
| invalid message | 无效的消息格式 |
| device name required | 缺少设备名称 |
| local port required | 缺少本地端口 |
