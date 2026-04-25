# client/internal/appsvc

这个目录放客户端共享服务层。它把认证、设备、文件、任务和心跳能力封装成 GUI 可直接调用的方法，供 Windows 桌面端和 Android 端共同复用。

目录结构：

- `README.md`：本目录说明。
- `service.go`：共享服务对象和主要业务调用入口，包含平台可配置的客户端版本和有界心跳停止逻辑。
- `service_test.go`：共享服务层测试，覆盖配置、设备绑定和心跳停止行为。
