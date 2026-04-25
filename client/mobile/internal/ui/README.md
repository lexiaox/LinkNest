# client/mobile/internal/ui

这个目录放 Android 界面实现。当前基于 Fyne，为手机端提供账号、设备、文件和上传任务四个页签。

移动端 UI 需要优先控制最小宽度：账号页不使用 Fyne `Form`，而是用短标签和忽略子控件超宽最小值的竖向布局；服务器地址输入框使用可换行的一行起步输入，状态、快照和列表项里的长 URL、ID、路径、文件名使用按字符换行或缩短展示。设备页通过共享服务层只展示在线设备，避免 DHCP 历史离线设备挤满手机列表；设备、文件和上传任务页使用顶部控制区加中间列表区的布局，全局状态显示保持在内容顶部，避免列表内容被底部页签或外置状态区挤压遮挡。

V2 能力通过共享 client service 接入：设备页启动/停止 P2P 服务，文件页选择在线目标设备并发送文件，任务页同时展示 V2 transfer 和 V1 upload task。文件下载优先写入系统公共 Downloads 目录：`/storage/emulated/0/Download`，旧设备会尝试 `/sdcard/Download`。如果系统权限或厂商 ROM 拒绝写入公共目录，才回退到应用沙箱 `Documents` 目录，并在文件页提示实际保存路径。

文件下载优先写入系统公共 Downloads 目录：`/storage/emulated/0/Download`，旧设备会尝试 `/sdcard/Download`。如果系统权限或厂商 ROM 拒绝写入公共目录，才回退到应用沙箱 `Documents` 目录，并在文件页提示实际保存路径。

目录结构：

- `app_android.go`：Android GUI 的窗口、页签和交互逻辑，包含 P2P 服务开关、目标设备发送、V2/V1 任务列表和系统 Downloads 下载路径。
