# server

服务端目录，承载 LinkNest V1 的 HTTP API、WebSocket 心跳、数据库迁移和内置 Web 资源。

## 文件结构

- `README.md`：本目录说明。
- `cmd/`：服务端可执行入口。
- `internal/`：服务端内部模块实现。
- `migrations/`：SQLite 迁移脚本。
- `web/`：内置静态页面和模板占位。
