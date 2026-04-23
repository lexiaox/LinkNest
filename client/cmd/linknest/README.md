# client/cmd/linknest

CLI 主程序目录，负责解析用户命令并调用内部模块。

## 面向用户的常用命令

- `linknest setup --username demo --password password`
  适用于已有账号，一步完成登录、设备初始化和设备注册。
- `linknest setup --register --username demo --email demo@example.com --password password`
  适用于新用户，一步完成注册、登录、设备初始化和设备注册。
- `linknest online`
  启动设备心跳，让当前设备持续显示为在线。

底层命令 `auth ...`、`device ...`、`file ...`、`task ...` 仍然保留，方便调试和脚本化使用。

## 文件结构

- `README.md`：本目录说明。
- `main.go`：LinkNest CLI 入口和命令分发。
