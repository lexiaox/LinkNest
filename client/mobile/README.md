# client/mobile

这个目录放 LinkNest 的移动端代码。当前先实现 Android GUI，复用现有客户端模块和共享服务层，不再通过 CLI 子进程调用。

当前实现基于 Fyne，重点覆盖账号、设备、文件和上传任务四个页面。移动端布局按手机屏幕做了单列化调整，账号页使用短标签和强制按屏幕宽度排布的竖向表单，长 URL、ID、路径和文件名会按字符换行或缩短展示，避免把页面横向撑出屏幕；列表页的按钮区固定在上方，列表占用中间剩余空间；全局状态显示放在内容顶部，避免挤占底部页签空间。

Android 构建说明：

- 入口源码：`./client/mobile/cmd/linknest-mobile`
- 生成 Android 安装包时，推荐按 Fyne 官方方式用 `fyne package -os android`
- 当前 Android 端会把本地配置和上传临时文件放在应用沙箱内；下载文件会优先保存到系统公共 Downloads 目录，系统拒绝写入时才回退到应用沙箱 `Documents` 目录

目录结构：

- `cmd/`：移动端可执行入口。
- `internal/`：移动端界面实现。
