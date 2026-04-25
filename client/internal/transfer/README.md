# client/internal/transfer

传输客户端目录，实现 V1 云端分片上传/下载/恢复，以及 V2 P2P 优先、V1 云端兜底的 transfer send/list/detail/resume/fallback 能力。

## 文件结构

- `README.md`：本目录说明。
- `transfer.go`：V1 文件、上传任务、分片上传、下载、删除和恢复能力。
- `v2.go`：V2 传输调度客户端、P2P 探测、分片直传、cloud fallback 和本地 transfer 缓存。
