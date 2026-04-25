# LinkNest API

本文档记录当前仓库已经落地的接口。

## 已实现接口

### 健康检查

- `GET /healthz`

响应：

```json
{
  "status": "ok"
}
```

### 认证

- `POST /api/auth/register`
- `POST /api/auth/login`
- `GET /api/auth/me`

登录和注册成功响应：

```json
{
  "token": "jwt-token",
  "user": {
    "id": 1,
    "username": "demo",
    "email": "demo@example.com"
  }
}
```

### 设备

- `POST /api/devices/register`
- `GET /api/devices`
- `GET /ws/devices?device_id={device_id}`

心跳消息：

```json
{
  "type": "heartbeat",
  "device_id": "device-uuid",
  "device_name": "demo-pc",
  "device_type": "linux",
  "lan_ip": "192.168.1.10",
  "port": 0,
  "p2p_enabled": true,
  "p2p_port": 19090,
  "p2p_protocol": "http",
  "virtual_ip": "100.64.0.10",
  "client_version": "0.1.0",
  "timestamp": "2026-04-23T16:00:00+08:00"
}
```

心跳响应：

```json
{
  "type": "heartbeat_ack",
  "server_time": "2026-04-23T16:00:05+08:00",
  "status": "online"
}
```

### 文件和任务

- `GET /api/files`
- `POST /api/files/init-upload`
- `GET /api/files/{file_id}/download`
- `GET /api/tasks`
- `GET /api/uploads/{upload_id}/missing-chunks`
- `PUT /api/uploads/{upload_id}/chunks/{chunk_index}`
- `POST /api/uploads/{upload_id}/complete`

初始化上传请求：

```json
{
  "device_id": "device-uuid",
  "file_name": "demo.zip",
  "file_size": 10485760,
  "file_hash": "sha256",
  "chunk_size": 4194304,
  "total_chunks": 3
}
```

### V2 传输调度

- `POST /api/transfers/init`
- `POST /api/transfers/validate-token`
- `GET /api/transfers`
- `GET /api/transfers/{transfer_id}/detail`
- `POST /api/transfers/{transfer_id}/probe-result`
- `POST /api/transfers/{transfer_id}/complete`
- `POST /api/transfers/{transfer_id}/fallback`

初始化传输请求：

```json
{
  "source_device_id": "source-device",
  "target_device_id": "target-device",
  "file_name": "demo.zip",
  "file_size": 10485760,
  "file_hash": "sha256",
  "chunk_size": 4194304,
  "total_chunks": 3
}
```

目标在线且支持 P2P 时返回：

```json
{
  "transfer_id": "transfer-uuid",
  "preferred_route": "p2p",
  "fallback_route": "cloud",
  "transfer_token": "short-lived-token",
  "p2p_candidates": [
    {
      "host": "192.168.1.20",
      "port": 19090,
      "protocol": "http",
      "network_type": "lan"
    }
  ],
  "status": "initialized"
}
```

目标离线或不支持 P2P 时，`preferred_route` 为 `cloud`，状态为 `fallback_uploading`，客户端复用 V1 `init-upload`、分片上传、`complete` 和下载链路。

P2P 接收端本地 HTTP 接口：

- `POST /p2p/v1/probe`
- `PUT /p2p/v1/transfers/{transfer_id}/chunks/{chunk_index}`
- `POST /p2p/v1/transfers/{transfer_id}/complete`

缺失分片响应：

```json
{
  "upload_id": "upload-uuid",
  "file_id": "file-uuid",
  "total_chunks": 3,
  "uploaded_chunks": [0],
  "missing_chunks": [1, 2],
  "status": "uploading"
}
```

完成上传成功响应：

```json
{
  "upload_id": "upload-uuid",
  "file_id": "file-uuid",
  "status": "completed"
}
```

## 统一错误格式

```json
{
  "error": {
    "code": "AUTH_INVALID_TOKEN",
    "message": "invalid token"
  }
}
```
