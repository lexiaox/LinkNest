# client/cmd/linknest

CLI 主程序目录，负责解析用户命令并调用内部模块。

## 面向用户的常用命令

- `linknest setup --username demo --password password`
  适用于已有账号，一步完成登录、设备初始化和设备注册。
- `linknest setup --register --username demo --email demo@example.com --password password`
  适用于新用户，一步完成注册、登录、设备初始化和设备注册。
- `linknest online`
  启动设备心跳，让当前设备持续显示为在线。
- `linknest p2p serve`
  启动 V2 P2P HTTP 接收服务，并通过心跳上报监听端口。
- `linknest p2p status`
  查看本机 P2P、inbox 和 cloud fallback 配置。
- `linknest transfer send <path> --to <device_id>`
  向同账号在线设备发起 P2P 优先、云端兜底的 V2 传输。
- `linknest transfer list/detail/resume/fallback`
  查看、恢复或手动回退 V2 传输任务。
- `linknest device list`
  查看当前账号下的在线设备；离线设备默认隐藏。

底层命令 `auth ...`、`device ...`、`file ...`、`task ...`、`p2p ...`、`transfer ...` 仍然保留，方便调试和脚本化使用。

## 文件结构

- `README.md`：本目录说明。
- `main.go`：LinkNest CLI 入口和命令分发。
