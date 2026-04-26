# server/migrations

数据库迁移目录，按顺序保存 SQLite 建表脚本。

## 文件结构

- `README.md`：本目录说明。
- `001_init.sql`：V1 初始化表结构。
- `002_v2_p2p_transfers.sql`：V2 P2P 设备字段和 `transfer_tasks` 调度表。
