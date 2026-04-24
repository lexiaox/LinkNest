# LinkNest

LinkNest 是一个可运行的多端文件传输原型，提供服务端、Web UI、CLI、Windows 桌面端和 Docker 部署方式。你可以把它部署到自己的服务器上，然后通过浏览器、CLI 或 Windows GUI 登录同一个账号、绑定设备、上传文件、下载文件、删除文件、查看任务状态，以及在不再需要时注销账号并清理自己的数据。

## 我能用它做什么

- 在自己的服务器上部署一个可访问的文件传输服务
- 用浏览器登录账号，查看设备、文件和任务
- 用 CLI 把当前电脑绑定成一个设备并保持在线
- 在 Windows 桌面端里完成登录、绑定设备、文件管理和上传任务查看
- 上传大文件、断点续传、补传缺失分片
- 在其他设备上查看文件、下载文件和删除文件
- 注销测试账号或不再使用的账号，清理对应的设备、文件和上传记录

## 作为部署者怎么启动服务

### 本地直接运行

1. 设置服务端密钥：

```bash
export LINKNEST_JWT_SECRET='replace-with-local-secret'
```

2. 启动服务端：

```bash
go run ./server/cmd/linknest-server --config ./deploy/config.example.yaml
```

3. 检查服务是否启动成功：

```bash
curl http://127.0.0.1:8080/healthz
```

如果返回 `{"status":"ok"}`，说明服务已经可用。

### 用 Docker Compose 部署

1. 设置服务端密钥：

```bash
export LINKNEST_JWT_SECRET='replace-with-server-secret'
```

2. 启动服务：

```bash
docker compose up -d --build
```

3. 检查状态：

```bash
docker compose ps
curl http://127.0.0.1:8080/healthz
```

### 作为服务器管理员，你至少会用到这些入口

- 登录页：`http://<server>:8080/login`
- 设备页：`http://<server>:8080/devices`
- 文件页：`http://<server>:8080/files`
- 任务页：`http://<server>:8080/tasks`
- 健康检查：`http://<server>:8080/healthz`

如果你部署时做了反向代理或端口映射，比如把公网 `80` 转发到容器 `8080`，把上面的 `:8080` 替换成你的实际访问地址即可。

## 作为普通用户怎么登录和使用 Web UI

1. 打开登录页：

```text
http://<server>:8080/login
```

2. 使用同一个账号注册或登录。

3. 登录后按页面使用：

- `Devices`：查看当前账号下的设备和在线状态
- `Files`：上传文件、查看文件列表、下载文件、删除文件
- `Tasks`：查看上传任务、续传状态和进度
- 页面右上角的 `注销账号`：输入当前密码后删除该账号及其设备、文件和上传记录

### 手机怎么用

- 手机当前通过浏览器访问同一个账号即可使用
- 当前是 Web 使用方式，不是原生 App
- 手机可以登录、查看设备、查看文件、上传下载
- 当前“正式设备绑定”主要由 CLI 客户端完成，手机浏览器更适合作为 Web 用户入口

## 作为 Windows 用户怎么使用桌面端

### 直接使用 release 里的可执行文件

1. 下载 GitHub Release 里的 Windows 压缩包。
2. 解压后运行 `linknest-desktop.exe`。
3. 在账号页先保存服务器地址，再登录或注册。
4. 在设备页绑定当前设备，并按需启动在线心跳。
5. 在文件页上传、下载和删除文件，在上传任务页查看任务状态。

### 自己编译 Windows 桌面端

需要：

- Go
- `CGO_ENABLED=1`
- 可用的 Windows C 编译器，例如 MSYS2 UCRT64 的 `gcc` / `g++`

编译命令：

```bash
go build -o ./bin/linknest-desktop.exe ./client/desktop/cmd/linknest-desktop
```

## 作为 CLI 用户怎么绑定设备和保持在线

### 新用户

```bash
go run ./client/cmd/linknest setup --register --username demo --email demo@example.com --password password
```

这条命令会一次性完成：

- 注册账号
- 登录账号
- 初始化当前设备
- 把当前设备注册到服务器

### 已有账号用户

```bash
go run ./client/cmd/linknest setup --username demo --password password
```

这条命令会一次性完成：

- 登录账号
- 初始化当前设备
- 把当前设备注册到服务器

### 保持设备在线

```bash
go run ./client/cmd/linknest online
```

这会持续发送设备心跳，让当前设备在设备页里显示为在线。

## 怎么上传、下载、删除文件、查看任务和注销账号

### 通过 Web UI

1. 进入文件页：`http://<server>:8080/files`
2. 选择文件后上传
3. 在任务页：`http://<server>:8080/tasks` 查看进度
4. 在文件页中下载目标文件
5. 在文件页中点击目标文件右侧的删除按钮，可以从服务器移除该文件
6. 如果不再需要当前账号，可以点击页面右上角的 `注销账号`，输入当前密码后清理该账号的数据

### 通过 CLI

上传文件：

```bash
go run ./client/cmd/linknest file upload ./demo.zip
```

查看文件列表：

```bash
go run ./client/cmd/linknest file list
```

下载文件：

```bash
go run ./client/cmd/linknest file download <file_id> --output ./downloaded-demo.zip
```

删除文件：

```bash
go run ./client/cmd/linknest file delete <file_id>
```

查看任务列表：

```bash
go run ./client/cmd/linknest task list
```

继续未完成任务：

```bash
go run ./client/cmd/linknest task resume <upload_id>
```

注销当前账号：

```bash
go run ./client/cmd/linknest auth delete --password <当前密码>
```

执行后会：

- 删除当前账号下的设备记录
- 删除当前账号下的文件和上传任务
- 清理服务器上的用户文件存储目录和分片目录
- 清空本地 CLI 保存的登录 token

## 常见访问入口

- 登录页：`/login`
- 设备页：`/devices`
- 文件页：`/files`
- 任务页：`/tasks`
- 健康检查：`/healthz`

## 默认路径

- 本地数据库：`./data/linknest.db`
- 本地文件存储：`./data/storage`
- 本地分片目录：`./data/chunks`
- Docker 数据库：`/var/lib/linknest/linknest.db`
- Docker 文件存储：`/var/lib/linknest/storage`
- Docker 分片目录：`/var/lib/linknest/chunks`

## 补充文档入口

- `client/`：CLI 客户端代码和模块说明
- `client/desktop/`：Windows 桌面端代码和构建说明
- `server/`：服务端代码、迁移脚本和 Web 资源
- `deploy/`：本地和 Docker 配置模板
- `docs/api.md`：API 说明
- `docs/dev-plan.md`：开发计划

各级目录下也都带有 `README.md`，用于说明该目录的职责、结构和文件用途。
