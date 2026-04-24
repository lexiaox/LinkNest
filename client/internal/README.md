# client/internal

客户端内部模块目录，按认证、配置、设备、传输和 WebSocket 拆分。这些模块同时被 CLI、Windows 桌面端和 Android 移动端复用。

## 文件结构

- `README.md`：本目录说明。
- `appsvc/`：桌面端和移动端共用的客户端服务层。
- `auth/`：认证接口调用。
- `config/`：本地配置读写。
- `device/`：设备初始化、本地设备文件、设备接口调用。
- `transfer/`：文件和任务相关客户端能力。
- `websocket/`：设备心跳客户端，支持显式停止心跳。
