# server/internal/httpapi

HTTP API 组装层，负责把各个内部服务暴露为 HTTP 路由。

## 文件结构

- `README.md`：本目录说明。
- `router.go`：服务端路由和处理器。
- `router_test.go`：路由层辅助逻辑测试，覆盖 Web 设备页使用的在线设备过滤。
