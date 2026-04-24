# client

客户端目录，包含 CLI 和 Windows 桌面端两条使用链路。CLI 负责命令行配置、设备初始化、认证调用和心跳发送；桌面端在此基础上提供 Windows GUI。

## 文件结构

- `README.md`：本目录说明。
- `cmd/`：CLI 可执行入口。
- `desktop/`：Windows 桌面端入口、服务层和界面实现。
- `internal/`：客户端内部模块。
