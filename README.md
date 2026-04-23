# LinkNest

LinkNest 是按《LinkNest 项目方案书 V1》落地的本地开发仓库。当前版本已经生成了 V1 对应的工程骨架，并实现了服务端基础、用户认证、设备注册、设备列表、WebSocket 心跳、分片上传闭环、文件下载和 CLI 原型。

## 当前已生成的范围

- 服务端基础能力：YAML 配置、SQLite 初始化、迁移执行、统一错误响应、`/healthz`。
- 用户认证：注册、登录、当前用户接口，Bearer Token 鉴权。
- 设备管理：设备初始化、设备注册、设备列表、在线状态更新。
- 在线状态：WebSocket 心跳接收、`last_seen_at` 更新、后台离线扫描。
- 文件链路：`init-upload`、缺失分片查询、单分片上传、合并校验、文件列表和下载。
- CLI 原型：`auth register/login`、`device init/register/list/heartbeat`、`file upload/list/download`、`task list/resume`。
- 文档与目录说明：每一层目录都带 `README.md`，说明职责、结构和文件用途。

## 尚未完成的范围

- 上传任务的更细粒度进度展示和失败重试体验。
- 基础 Web UI 的业务页面。
- 更完整的自动化测试覆盖，特别是 API 层和 CLI 恢复场景。

这些模块已经按 V1 方案书预留了目录、数据表和代码边界，后续可以继续往下补。

## 根目录结构

- `go.mod`：Go 模块定义和依赖版本。
- `README.md`：项目总览、运行方式和目录导航。
- `server/`：服务端代码、迁移脚本和内置静态资源。
- `client/`：CLI 客户端代码。
- `docs/`：接口说明和开发推进文档。
- `deploy/`：本地配置模板。
- `Dockerfile`：服务端镜像构建文件。
- `docker-compose.yml`：服务端容器部署模板。

## 快速运行

1. 设置环境变量：

```bash
export LINKNEST_JWT_SECRET='replace-with-local-secret'
```

2. 启动服务端：

```bash
go run ./server/cmd/linknest-server --config ./deploy/config.example.yaml
```

3. 初始化客户端配置和设备：

```bash
go run ./client/cmd/linknest auth register --username demo --email demo@example.com --password password
go run ./client/cmd/linknest auth login --username demo --password password
go run ./client/cmd/linknest device init --name demo-pc --type linux
go run ./client/cmd/linknest device register
go run ./client/cmd/linknest device heartbeat
```

4. 上传和下载文件：

```bash
go run ./client/cmd/linknest file upload ./demo.zip
go run ./client/cmd/linknest file list
go run ./client/cmd/linknest file download <file_id> --output ./downloaded-demo.zip
go run ./client/cmd/linknest task list
go run ./client/cmd/linknest task resume <upload_id>
```

## Docker 部署

### 直接构建并运行

1. 构建镜像：

```bash
docker build -t linknest-server:latest .
```

2. 启动容器：

```bash
docker run -d \
  --name linknest-server \
  --restart unless-stopped \
  -p 8080:8080 \
  -e LINKNEST_JWT_SECRET='replace-with-server-secret' \
  -v linknest-data:/var/lib/linknest \
  linknest-server:latest
```

### 直接发到服务器上部署

把整个 `~/LinkNest` 发到服务器后，在服务器目录里执行：

```bash
export LINKNEST_JWT_SECRET='replace-with-server-secret'
docker build -t linknest-server:latest .
docker rm -f linknest-server 2>/dev/null || true
docker run -d \
  --name linknest-server \
  --restart unless-stopped \
  -p 8080:8080 \
  -e LINKNEST_JWT_SECRET="$LINKNEST_JWT_SECRET" \
  -v linknest-data:/var/lib/linknest \
  linknest-server:latest
```

### 检查服务

```bash
curl http://127.0.0.1:8080/healthz
docker logs -f linknest-server
```

## 说明

- 默认数据库路径是 `./data/linknest.db`。
- 默认文件存储路径是 `./data/storage` 和 `./data/chunks`。
- Docker 默认数据库路径是 `/var/lib/linknest/linknest.db`。
- Docker 默认文件存储路径是 `/var/lib/linknest/storage` 和 `/var/lib/linknest/chunks`。
- 服务端首页会返回一个简单的 V1 状态页，完整业务页面后续补充。

更多目录级说明见各级 `README.md`。
