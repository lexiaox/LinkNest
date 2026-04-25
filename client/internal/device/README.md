# client/internal/device

设备客户端目录，负责生成稳定设备 ID、管理 `device.json`、调用设备接口，并提供在线设备过滤逻辑。V2 心跳会携带 P2P 开关、监听端口、协议和 virtual IP，让服务端可以生成直连候选地址。

## 文件结构

- `README.md`：本目录说明。
- `device.go`：设备本地文件和服务端接口封装，包含 `OnlineOnly` / `IsOnline`、P2P 心跳 payload 和远端设备 P2P 元数据。
- `device_test.go`：设备列表过滤规则测试。
