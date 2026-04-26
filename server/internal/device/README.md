# server/internal/device

设备模块，负责设备注册、设备列表和在线状态维护。V2 扩展设备心跳和 `devices` 表字段，用于保存 P2P 开关、监听端口、协议、LAN IP、virtual IP 和最近 P2P 上报时间。

## 文件结构

- `README.md`：本目录说明。
- `service.go`：设备模型和数据库操作，包含 P2P 元数据读写和按账号隔离的设备查询。
