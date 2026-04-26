# server/internal/httpapi

HTTP API 组装层，负责把各个内部服务暴露为 HTTP 路由。V2 新增 `/api/transfers/*` 调度接口和 `/api/transfers/validate-token`，供 CLI/GUI 与 P2P 接收服务完成初始化、探测结果、完成、回退、列表和详情查询。

## 文件结构

- `README.md`：本目录说明。
- `router.go`：服务端路由和处理器，包含认证、设备、文件、上传任务、在线设备过滤和 V2 transfer API。
