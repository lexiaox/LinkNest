# server/internal/transfer

V2 传输调度目录，负责创建 P2P 优先、云端兜底的传输任务，并记录探测、完成和失败回退状态。

## 文件结构

- `README.md`：本目录说明。
- `service.go`：传输任务服务、候选地址生成、状态流转和短期 token 校验入口。
- `service_test.go`：P2P/cloud 路由选择和 transfer token 设备匹配测试。
