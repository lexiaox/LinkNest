# client/internal/p2p

V2 P2P 接收服务目录，负责本机 HTTP 监听、传输 token 校验、分片接收、文件合并和接收端任务缓存。

## 文件结构

- `README.md`：本目录说明。
- `server.go`：P2P HTTP 接收服务和本地任务缓存，暴露 `/p2p/v1/probe`、分片接收和 complete 合并接口。
