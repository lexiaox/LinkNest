# client/desktop/cmd/linknest-desktop

这个目录提供 LinkNest Windows GUI 的程序入口。当前入口会解析本地客户端配置目录，然后启动桌面应用窗口。

目录结构：

- `main_windows.go`：Windows 桌面端入口。

运行结果：

- 本地构建后会得到 `linknest-desktop.exe`
- GitHub Release 的 Windows 压缩包也会提供这个可执行文件
