# client/mobile

这个目录放 LinkNest 的移动端代码。当前先实现 Android GUI，复用现有客户端模块和共享服务层，不再通过 CLI 子进程调用。

当前实现基于 Fyne，重点覆盖账号、设备、文件和上传任务四个页面，布局按手机屏幕做了单列化调整。

Android 构建说明：

- 入口源码：`./client/mobile/cmd/linknest-mobile`
- 生成 Android 安装包时，推荐按 Fyne 官方方式用 `fyne package -os android`
- 当前 Android 端会把本地配置和临时下载目录放在应用沙箱内

目录结构：

- `cmd/`：移动端可执行入口。
- `internal/`：移动端界面实现。
