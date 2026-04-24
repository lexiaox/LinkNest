# client/desktop/internal/appsvc

这个目录封装桌面端服务层。它负责把现有的认证、设备、文件、任务和心跳能力包装成 GUI 可直接调用的方法。

目录结构：

- `service.go`：桌面端服务对象和主要业务调用入口。
- `service_test.go`：服务层的最小测试。
