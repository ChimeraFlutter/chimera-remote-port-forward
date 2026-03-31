# 设计文档

## 项目概述

chimera-remote-port-forward 是一个端口映射服务，支持将本地 TCP 端口映射到服务端的公网端口。

### 核心功能

- 服务端管理端口池 (10000-11000，可配置)
- 客户端通过 WebSocket 连接服务端
- 客户端申请端口后，将本地端口映射到服务端分配的端口
- 客户端发送设备名称标识
- 服务端提供网页查看所有映射
- 心跳保持连接
- 仅支持 TCP

## 使用方式

### 服务端启动

```bash
./chimera-remote-port-forward -mode server

# 输出示例:
# WebSocket listening on: :52341
# Web interface on: http://localhost:52342
# Port range: 10000-11000
```

### 客户端启动

```bash
./chimera-remote-port-forward -mode client \
    -server ws://your-server:52341/ws \
    -device-name "my-laptop" \
    -local-port 3000 \
    -token "your-secret-token"
```

## 端口说明

| 服务 | 端口 | 说明 |
|------|------|------|
| WebSocket | 52341 | 客户端连接端口 |
| Web 界面 | 52342 | 管理界面 |
| 映射端口 | 10000-11000 | 动态分配的端口范围 |

## 工作流程

1. **服务端启动**
   - 监听 WebSocket 端口 (52341)
   - 启动 Web 管理界面 (52342)
   - 初始化端口池 (10000-11000)

2. **客户端连接**
   - 通过 WebSocket 连接服务端
   - 发送 Token 认证
   - 发送设备名称和本地端口
   - 服务端动态分配一个端口

3. **端口映射**
   - 外部请求 → 服务端分配端口 → WebSocket → 客户端 → 本地端口
   - 仅支持 TCP

4. **心跳保持**
   - 客户端每30秒发送心跳
   - 断线后每3秒自动重连

5. **Web界面**
   - 访问 http://server:52342
   - 需要密码登录
   - 查看所有设备映射状态
