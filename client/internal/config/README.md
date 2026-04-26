# client/internal/config

本地配置目录，负责管理 `~/.linknest` 下的配置文件、V1 上传任务目录、V2 传输任务目录和 P2P 接收 inbox。

V2 新增的 `transfer` 配置包含 chunk size、P2P 开关、监听 host/port、连接超时、重试次数、cloud fallback 开关、inbox 路径和 virtual IP。`fallback_to_cloud` 和 `p2p_enabled` 使用可区分默认值和显式 false 的布尔配置。

## 文件结构

- `README.md`：本目录说明。
- `config.go`：客户端配置读写、目录初始化、P2P/fallback 默认值归一化。
