# server/internal/websocket

WebSocket 模块，负责接收设备心跳并刷新在线状态。V2 心跳会同步 P2P 开关、端口、协议和 virtual IP，供服务端 transfer 调度生成候选地址。

## 文件结构

- `README.md`：本目录说明。
- `hub.go`：WebSocket 心跳处理器，负责鉴权、设备归属校验、在线状态刷新和 P2P 元数据入库。
