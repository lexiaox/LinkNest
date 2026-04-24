# client/desktop

这个目录放 LinkNest 的桌面端代码。当前目标是提供一个 Windows GUI，把现有 CLI 的认证、设备、文件和任务能力封装成可视化界面。

当前实现基于 Fyne，桌面端主要复用 `client/internal/*` 里的现有 Go 逻辑，而不是重新实现一套 HTTP 或 CLI 包装层。

Windows 原生构建说明：

- 入口：`go build ./client/desktop/cmd/linknest-desktop`
- 由于 Fyne 的 Windows 图形栈依赖 `CGO`，本机需要可用的 `gcc`/`g++` 工具链
- 如果本机只有 Go 而没有 C 编译器，WSL 侧 `go test ./...` 仍然可以通过，但 Windows GUI 可执行文件无法直接编出来

目录结构：

- `cmd/`：桌面应用入口。
- `internal/`：桌面端服务层和界面实现。
