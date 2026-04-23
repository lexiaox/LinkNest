# server/internal

服务端内部实现目录，按 V1 方案书拆成认证、设备、文件、任务、存储、WebSocket 等模块。

## 文件结构

- `README.md`：本目录说明。
- `auth/`：用户认证和 token 处理。
- `config/`：服务端配置解析与校验。
- `database/`：数据库连接和迁移执行。
- `device/`：设备注册、查询和在线状态更新。
- `file/`：文件元数据查询与后续上传入口预留。
- `httpapi/`：HTTP 路由组装和处理器。
- `middleware/`：日志、恢复、鉴权中间件。
- `response/`：统一 JSON 响应封装。
- `storage/`：本地磁盘路径抽象。
- `task/`：上传任务查询与后续恢复逻辑预留。
- `websocket/`：设备心跳处理。
