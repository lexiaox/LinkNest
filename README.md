# LinkNest

LinkNest 是一个面向动态网络环境的多端数据传输与云端资源调度系统。本仓库当前已经按《LinkNest 项目方案书 V1》落地出可运行的服务端、CLI 和基础 Web UI，用于验证设备注册、在线状态维护、分片上传、断点续传和文件下载这条主链路。

## 当前状态

项目处于 V1 可运行原型阶段，已经完成核心工程骨架和主要业务闭环：

- 用户注册、登录和 Bearer Token 鉴权
- 多设备绑定与稳定 Device ID
- 设备 WebSocket 心跳与在线状态维护
- 文件初始化上传、缺失分片查询、分片补传、合并校验和下载
- Go CLI 原型
- 基础 Web UI：登录页、设备页、文件页、任务页
- Docker 打包与容器部署

## V1 目标与边界

V1 优先完成这些能力：

- 同一账号下多设备注册和在线管理
- 文件上传到云端资源池并保存元数据
- 大文件分片上传、SHA-256 校验和断点续传
- 其他设备查看文件列表并下载
- 用 CLI 和 Web UI 验证完整链路

V1 暂不实现：

- 局域网 P2P 直连传输
- WebRTC / QUIC 传输
- 完整文件夹同步
- 端到端加密
- AI 文件助手
- Flutter 多端客户端

## 技术方向

| 层级 | 当前实现 | 作用 |
| --- | --- | --- |
| 服务端 | Go 单体服务 | 用户、设备、文件、上传任务、Web UI |
| CLI 客户端 | Go | 认证、设备注册、上传下载、任务恢复 |
| Web UI | 原生 HTML / CSS / JS | 登录、设备、文件、任务状态页 |
| 数据库 | SQLite | 用户、设备、文件、任务和分片元数据 |
| 文件存储 | 本地磁盘 | 临时分片和合并后的文件 |

## 目录结构

- `server/`：服务端代码、迁移脚本和内置静态资源
- `client/`：CLI 客户端代码
- `docs/`：API 文档和开发计划
- `deploy/`：本地与 Docker 配置模板
- `Dockerfile`：服务端镜像构建文件
- `docker-compose.yml`：容器部署模板

每一层目录都带有 `README.md`，用于说明目录职责、文件结构和文件用途。

## 快速运行

1. 设置环境变量：

```bash
export LINKNEST_JWT_SECRET='replace-with-local-secret'
```

2. 启动服务端：

```bash
go run ./server/cmd/linknest-server --config ./deploy/config.example.yaml
```

3. 初始化账号和设备：

```bash
go run ./client/cmd/linknest setup --register --username demo --email demo@example.com --password password
go run ./client/cmd/linknest online
```

如果账号已经存在，可以直接：

```bash
go run ./client/cmd/linknest setup --username demo --password password
go run ./client/cmd/linknest online
```

旧的 `auth ...` 和 `device ...` 分步命令仍然保留，主要用于调试和脚本化场景。

4. 上传和下载文件：

```bash
go run ./client/cmd/linknest file upload ./demo.zip
go run ./client/cmd/linknest file list
go run ./client/cmd/linknest file download <file_id> --output ./downloaded-demo.zip
go run ./client/cmd/linknest task list
go run ./client/cmd/linknest task resume <upload_id>
```

## Web UI

服务启动后可直接访问：

- `/login`
- `/devices`
- `/files`
- `/tasks`

## Docker 部署

### 直接构建并运行

```bash
docker build -t linknest-server:latest .
docker run -d \
  --name linknest-server \
  --restart unless-stopped \
  -p 8080:8080 \
  -e LINKNEST_JWT_SECRET='replace-with-server-secret' \
  -v linknest-data:/var/lib/linknest \
  linknest-server:latest
```

### 使用 Compose

```bash
export LINKNEST_JWT_SECRET='replace-with-server-secret'
docker compose up -d --build
```

## 默认路径

- 本地数据库：`./data/linknest.db`
- 本地文件存储：`./data/storage`
- 本地分片目录：`./data/chunks`
- Docker 数据库：`/var/lib/linknest/linknest.db`
- Docker 文件存储：`/var/lib/linknest/storage`
- Docker 分片目录：`/var/lib/linknest/chunks`

## 协作约定

- `main` 分支保持可用状态
- 功能改动通过 Pull Request 合并
- 不提交密钥、token、`.env`、私钥或本地数据库文件
- 涉及行为变化时同步更新 README、API 文档或开发计划

更多细节见各级目录下的 `README.md` 与 `docs/` 文档。
