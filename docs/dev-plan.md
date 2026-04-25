# LinkNest V2 开发推进

本文档把 V2 方案书中的开发顺序映射到当前仓库状态。

## 当前已完成

1. 保留 V1 登录、设备、文件、上传任务、断点续传、下载、删除和账号注销能力。
2. 服务端扩展 `devices` 心跳字段，保存 P2P 开关、端口、协议、LAN IP 和 virtual IP。
3. 新增 `transfer_tasks` 调度表、短期 P2P transfer token 和 `/api/transfers/*` API。
4. CLI 新增 `p2p serve/status` 和 `transfer send/list/detail/resume/fallback`。
5. CLI P2P 接收服务支持 probe、分片接收、chunk hash、整体 hash、合并文件和任务缓存。
6. 失败时默认回退到 V1 云端分片上传链路；禁用 fallback 时返回失败。
7. Web 设备页展示 P2P 能力和候选地址类型，任务页展示 V2 P2P/cloud 传输任务。
8. Windows 和 Android GUI 通过共享 client service 接入 P2P 服务开关、目标设备发送和 V2 任务详情。

## 当前不做

1. 公网 NAT 打洞、TURN 中继、QUIC/WebRTC。
2. 端到端加密、文件夹同步、多人共享。
3. Web UI 作为本地 P2P 接收服务；Web 仅作为管理和诊断入口。

## 下一步建议

1. 在具备两个 CLI root 和两个 P2P 端口的环境中跑端到端 P2P 直传验收。
2. 补充 transfer send 中断重试、缺失分片恢复和 hash 失败回退的集成测试。
3. 在具备 Windows cgo/OpenGL 和 Android SDK/NDK 的机器上构建 release GUI 产物。
4. 根据真实局域网测试结果再决定是否加入更完整的候选地址优先级和连接失败诊断。
