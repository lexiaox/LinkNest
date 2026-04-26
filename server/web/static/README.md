# server/web/static

静态资源目录，提供内置 Web UI 的页面文件和共享前端资源。设备页只展示在线设备，并展示 P2P 能力、端口和候选地址类型；任务页同时展示 V1 上传任务和 V2 P2P/cloud 传输任务。

## 文件结构

- `README.md`：本目录说明。
- `index.html`：Web UI 导航首页。
- `login.html`：登录与注册页面。
- `devices.html`：设备列表页。
- `files.html`：文件列表与浏览器上传页。
- `tasks.html`：上传任务页。
- `app.css`：共享样式，包含 V2 任务分区标题和 P2P 状态标签样式。
- `app.js`：共享前端逻辑与 API 调用，包含设备 P2P 元数据展示和 transfer task 诊断表。
