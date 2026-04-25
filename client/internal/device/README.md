# client/internal/device

设备客户端目录，负责生成稳定设备 ID、管理 `device.json`、调用设备接口，并提供在线设备过滤逻辑。

## 文件结构

- `README.md`：本目录说明。
- `device.go`：设备本地文件和服务端接口封装，包含 `OnlineOnly` / `IsOnline` 用于让客户端只展示在线设备。
- `device_test.go`：设备列表过滤规则测试。
