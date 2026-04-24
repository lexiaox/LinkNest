# client/internal/websocket

客户端 WebSocket 目录，负责持续发送设备心跳并处理服务端 ack。当前实现支持显式停止信号，避免 GUI 停止心跳时被阻塞。

## 文件结构

- `README.md`：本目录说明。
- `heartbeat.go`：设备心跳客户端实现，包含可取消的 `RunHeartbeatUntil`。
- `heartbeat_test.go`：心跳取消与健康连接退出的回归测试。
