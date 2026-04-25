# server/internal/p2ptoken

短期 P2P 传输 token 目录，负责为服务端调度生成和校验单次传输使用的 JWT。

## 文件结构

- `README.md`：本目录说明。
- `token.go`：P2P transfer token 的签发和校验逻辑。
- `token_test.go`：token 签发、解析和过期拒绝测试。
