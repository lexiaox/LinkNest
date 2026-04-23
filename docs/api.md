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
