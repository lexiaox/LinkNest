# client/desktop/internal/ui

这个目录放桌面界面实现。当前基于 Fyne，为 Windows 提供账号、设备、文件和上传任务四个页签。设备页通过共享服务层只展示在线设备，离线设备不会占用列表空间。

目录结构：

- `app_windows.go`：Windows GUI 的窗口、页签和交互逻辑，包含后台状态提示、自动刷新、上传任务详情和文件操作按钮。桌面端应用标识使用中性的 `org.linknest.desktop`，不绑定个人域名。
