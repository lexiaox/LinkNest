const TOKEN_KEY = "linknest.token";
const USER_KEY = "linknest.user";
const UPLOAD_CACHE_KEY = "linknest.webUploadTasks";
const FLASH_KEY = "linknest.flash";

function getToken() {
  return window.localStorage.getItem(TOKEN_KEY) || "";
}

function setToken(token) {
  window.localStorage.setItem(TOKEN_KEY, token);
}

function clearSession() {
  window.localStorage.removeItem(TOKEN_KEY);
  window.localStorage.removeItem(USER_KEY);
}

function setFlashMessage(message) {
  if (!message) {
    window.sessionStorage.removeItem(FLASH_KEY);
    return;
  }
  window.sessionStorage.setItem(FLASH_KEY, message);
}

function consumeFlashMessage() {
  const message = window.sessionStorage.getItem(FLASH_KEY) || "";
  window.sessionStorage.removeItem(FLASH_KEY);
  return message;
}

function setUser(user) {
  window.localStorage.setItem(USER_KEY, JSON.stringify(user || {}));
}

function getUser() {
  const raw = window.localStorage.getItem(USER_KEY);
  if (!raw) {
    return null;
  }
  try {
    return JSON.parse(raw);
  } catch (error) {
    return null;
  }
}

function getUploadCache() {
  const raw = window.localStorage.getItem(UPLOAD_CACHE_KEY);
  if (!raw) {
    return [];
  }
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (error) {
    return [];
  }
}

function saveUploadCache(items) {
  window.localStorage.setItem(UPLOAD_CACHE_KEY, JSON.stringify(items || []));
}

function upsertUploadCache(task) {
  const items = getUploadCache().filter((item) => item.uploadId !== task.uploadId);
  items.unshift(task);
  saveUploadCache(items.slice(0, 12));
}

function removeUploadCache(uploadId) {
  saveUploadCache(getUploadCache().filter((item) => item.uploadId !== uploadId));
}

function removeUploadCacheByFileId(fileId) {
  saveUploadCache(getUploadCache().filter((item) => item.fileId !== fileId));
}

function getUploadTask(uploadId) {
  return getUploadCache().find((item) => item.uploadId === uploadId) || null;
}

function isLiveUploadTask(task) {
  const status = String((task && task.status) || "").toLowerCase();
  return ["initialized", "uploading", "failed"].includes(status);
}

function escapeHTML(value) {
  return String(value == null ? "" : value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

async function apiFetch(url, options = {}) {
  const headers = new Headers(options.headers || {});
  const token = getToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(url, {
    ...options,
    headers,
  });

  const contentType = response.headers.get("content-type") || "";
  let body = null;
  if (contentType.includes("application/json")) {
    body = await response.json();
  } else if (options.expectBlob) {
    body = await response.blob();
  } else {
    body = await response.text();
  }

  if (!response.ok) {
    const errorMessage =
      body && body.error && body.error.message
        ? `${body.error.code}: ${body.error.message}`
        : `请求失败: ${response.status}`;
    throw new Error(errorMessage);
  }

  return { response, body };
}

async function ensureAuth() {
  if (!getToken()) {
    window.location.href = "/login";
    return null;
  }

  try {
    const result = await apiFetch("/api/auth/me");
    if (result.body && result.body.user) {
      setUser(result.body.user);
      return result.body.user;
    }
  } catch (error) {
    clearSession();
    window.location.href = "/login";
    return null;
  }
  return null;
}

function renderShell(activeKey, pageTitle, pageCopy) {
  const app = document.getElementById("app");
  const user = getUser();
  const userLabel = user && user.username ? `当前用户：${escapeHTML(user.username)}` : "未登录";

  app.innerHTML = `
    <div class="app-shell">
      <header class="topbar">
        <div class="brand">
          <span class="brand-mark">LinkNest V1</span>
          <strong class="brand-title">${escapeHTML(pageTitle)}</strong>
          <span class="brand-subtitle">${escapeHTML(pageCopy)}</span>
        </div>
        <div class="topbar-actions">
          <span class="user-chip">${userLabel}</span>
          <a class="button ghost" href="/">首页</a>
          <button id="delete-account-button" class="ghost" type="button">注销账号</button>
          <button id="logout-button" class="ghost" type="button">退出登录</button>
        </div>
      </header>
      <div class="layout-grid">
        <aside class="sidebar">
          <nav class="nav-list">
            ${navLink("/devices", "devices", activeKey, "设备页", "查看在线状态、局域网地址和最近心跳")}
            ${navLink("/files", "files", activeKey, "文件页", "浏览文件列表并通过浏览器直接上传")}
            ${navLink("/tasks", "tasks", activeKey, "任务页", "查看上传任务状态和分片进度")}
          </nav>
        </aside>
        <section class="content-panel">
          <div id="message-bar" class="message-bar"></div>
          <div id="page-content"></div>
        </section>
      </div>
    </div>
  `;

  document.getElementById("logout-button").addEventListener("click", () => {
    clearSession();
    window.location.href = "/login";
  });

  document.getElementById("delete-account-button").addEventListener("click", async () => {
    const password = window.prompt("输入当前密码以确认注销账号。");
    if (password == null) {
      return;
    }
    if (!password.trim()) {
      setMessage("需要输入当前密码。", "error");
      return;
    }

    const confirmed = window.confirm("确认注销当前账号吗？这会删除该账号下的设备、文件和上传记录。");
    if (!confirmed) {
      return;
    }

    try {
      setMessage("正在注销账号并清理服务器数据...", "info");
      await apiFetch("/api/auth/delete-account", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      clearSession();
      setFlashMessage("账号已注销，服务器中的相关数据已清理。");
      window.location.href = "/login";
    } catch (error) {
      setMessage(error.message, "error");
    }
  });
}

function navLink(href, key, activeKey, title, copy) {
  const activeClass = key === activeKey ? "active" : "";
  return `
    <a class="nav-link ${activeClass}" href="${href}">
      <strong>${escapeHTML(title)}</strong>
      <span>${escapeHTML(copy)}</span>
    </a>
  `;
}

function setMessage(message, kind = "info") {
  const bar = document.getElementById("message-bar");
  if (!bar) {
    return;
  }
  if (!message) {
    bar.className = "message-bar";
    bar.textContent = "";
    return;
  }
  bar.className = `message-bar visible ${kind}`;
  bar.textContent = message;
}

function setupAutoRefresh(refreshFn, intervalMs) {
  let inFlight = false;

  const run = async (trigger) => {
    if (inFlight) {
      return;
    }
    if (trigger === "auto" && document.visibilityState === "hidden") {
      return;
    }

    inFlight = true;
    try {
      await refreshFn(trigger);
    } finally {
      inFlight = false;
    }
  };

  const timerId = window.setInterval(() => {
    run("auto");
  }, intervalMs);

  const handleVisibility = () => {
    if (document.visibilityState === "visible") {
      run("visible");
    }
  };

  document.addEventListener("visibilitychange", handleVisibility);
  window.addEventListener(
    "beforeunload",
    () => {
      window.clearInterval(timerId);
      document.removeEventListener("visibilitychange", handleVisibility);
    },
    { once: true }
  );

  return { run };
}

function setAutoRefreshStatus(elementId, intervalMs, lastUpdatedAt) {
  const element = document.getElementById(elementId);
  if (!element) {
    return;
  }
  const intervalSeconds = Math.round(intervalMs / 1000);
  const lastText = lastUpdatedAt ? formatDate(lastUpdatedAt) : "等待首次刷新";
  element.textContent = `自动轮询 ${intervalSeconds} 秒 · 最近刷新 ${lastText}`;
}

function formatDate(value) {
  if (!value) {
    return "-";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString("zh-CN", { hour12: false });
}

function formatBytes(size) {
  if (size == null || Number.isNaN(size)) {
    return "-";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let current = size;
  let index = 0;
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024;
    index += 1;
  }
  return `${current.toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
}

function formatPercent(value) {
  if (!Number.isFinite(value)) {
    return "0%";
  }
  return `${Math.max(0, Math.min(100, value)).toFixed(0)}%`;
}

function statusPill(status) {
  const normalized = String(status || "unknown").toLowerCase();
  return `<span class="pill ${normalized}">${escapeHTML(normalized)}</span>`;
}

function renderSummaryCards(items) {
  return `
    <div class="summary-grid">
      ${items
        .map(
          (item) => `
            <article class="summary-card">
              <strong>${escapeHTML(item.label)}</strong>
              <div class="summary-value">${escapeHTML(item.value)}</div>
              <div class="summary-note">${escapeHTML(item.note)}</div>
            </article>
          `
        )
        .join("")}
    </div>
  `;
}

async function setupLoginPage() {
  if (getToken()) {
    window.location.href = "/devices";
    return;
  }

  const app = document.getElementById("app");
  app.innerHTML = `
    <div class="auth-page">
      <section class="auth-panel">
        <div class="auth-intro">
          <span class="hero-badge">Browser Control Surface</span>
          <h1>登录之后直接看设备、文件和上传任务</h1>
          <p>
            这个内置 Web UI 直接复用服务端 API。登录成功后，设备页看在线状态，文件页看列表并触发浏览器分片上传，任务页看上传进度。
          </p>
          <div class="auth-highlights">
            <div class="auth-highlight"><strong>设备页</strong><span>适合排查心跳、最近在线时间和局域网地址。</span></div>
            <div class="auth-highlight"><strong>文件页</strong><span>支持浏览器端按 4MB 分片上传并带 hash 校验。</span></div>
            <div class="auth-highlight"><strong>任务页</strong><span>展示 upload_id、状态、已上传分片数和失败信息。</span></div>
          </div>
        </div>
        <div class="auth-card">
          <div class="panel-tabs">
            <button id="tab-login" class="panel-tab active" type="button">登录</button>
            <button id="tab-register" class="panel-tab" type="button">注册</button>
          </div>
          <div id="login-message" class="message-bar"></div>
          <form id="auth-form" class="stack">
            <div class="field">
              <label for="username">用户名</label>
              <input id="username" name="username" autocomplete="username" required />
            </div>
            <div class="field" id="email-field" style="display:none">
              <label for="email">邮箱</label>
              <input id="email" name="email" autocomplete="email" />
            </div>
            <div class="field">
              <label for="password">密码</label>
              <input id="password" name="password" type="password" autocomplete="current-password" required />
            </div>
            <button class="primary" id="auth-submit" type="submit">登录并进入设备页</button>
          </form>
        </div>
      </section>
    </div>
  `;

  const state = { mode: "login" };
  const form = document.getElementById("auth-form");
  const emailField = document.getElementById("email-field");
  const submitButton = document.getElementById("auth-submit");
  const messageBar = document.getElementById("login-message");
  const flashMessage = consumeFlashMessage();

  function switchMode(mode) {
    state.mode = mode;
    const isRegister = mode === "register";
    document.getElementById("tab-login").classList.toggle("active", !isRegister);
    document.getElementById("tab-register").classList.toggle("active", isRegister);
    emailField.style.display = isRegister ? "grid" : "none";
    submitButton.textContent = isRegister ? "注册并进入设备页" : "登录并进入设备页";
    messageBar.className = "message-bar";
    messageBar.textContent = "";
  }

  document.getElementById("tab-login").addEventListener("click", () => switchMode("login"));
  document.getElementById("tab-register").addEventListener("click", () => switchMode("register"));

  if (flashMessage) {
    messageBar.className = "message-bar visible success";
    messageBar.textContent = flashMessage;
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const payload = {
      username: form.username.value.trim(),
      password: form.password.value,
    };
    if (state.mode === "register") {
      payload.email = form.email.value.trim();
    }

    try {
      messageBar.className = "message-bar visible info";
      messageBar.textContent = state.mode === "register" ? "正在注册..." : "正在登录...";
      const { body } = await apiFetch(`/api/auth/${state.mode}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      setToken(body.token);
      setUser(body.user);
      window.location.href = "/devices";
    } catch (error) {
      messageBar.className = "message-bar visible error";
      messageBar.textContent = error.message;
    }
  });
}

async function setupDevicesPage() {
  const user = await ensureAuth();
  if (!user) {
    return;
  }

  const refreshIntervalMs = 8000;
  renderShell("devices", "设备页", "检查在线状态、最近心跳和当前局域网连接信息。");
  const page = document.getElementById("page-content");
  page.innerHTML = `
    <section class="page-header">
      <div>
        <h1 class="page-title">设备视图</h1>
        <p class="page-copy">这个页面只读取在线设备。在线状态来自 WebSocket 心跳和服务端离线扫描。</p>
      </div>
      <div class="toolbar">
        <span id="devices-auto-status" class="user-chip">自动轮询准备中</span>
        <button id="refresh-devices" type="button">刷新设备列表</button>
      </div>
    </section>
    <div id="devices-summary"></div>
    <div id="devices-table" class="table-wrap"></div>
  `;

  const refresh = async (trigger = "manual") => {
    try {
      if (trigger === "manual" || trigger === "initial") {
        setMessage("正在刷新设备列表...", "info");
      }
      const { body } = await apiFetch("/api/devices?status=online");
      const items = body.items || [];
      document.getElementById("devices-summary").innerHTML = renderSummaryCards([
        { label: "在线设备", value: String(items.length), note: "仅显示当前在线设备" },
        { label: "数据来源", value: "online", note: "/api/devices?status=online" },
        { label: "刷新间隔", value: `${Math.round(refreshIntervalMs / 1000)}s`, note: "自动轮询最新心跳状态" },
      ]);
      document.getElementById("devices-table").innerHTML = renderDevicesTable(items);
      setAutoRefreshStatus("devices-auto-status", refreshIntervalMs, new Date());
      if (trigger === "manual" || trigger === "initial") {
        setMessage(`在线设备列表已刷新，用户 ${user.username} 当前有 ${items.length} 台在线设备。`, "success");
      }
    } catch (error) {
      setMessage(error.message, "error");
    }
  };

  document.getElementById("refresh-devices").addEventListener("click", () => refresh("manual"));
  await refresh("initial");
  setupAutoRefresh(refresh, refreshIntervalMs);
}

function renderDevicesTable(items) {
  if (!items.length) {
    return `<div class="empty-state">当前没有在线设备。请在设备上运行 <code>linknest online</code>，或打开 Windows / Android 客户端在线心跳。</div>`;
  }
  return `
    <table>
      <thead>
        <tr>
          <th>设备</th>
          <th>状态</th>
          <th>网络</th>
          <th>客户端</th>
          <th>最近在线</th>
        </tr>
      </thead>
      <tbody>
        ${items
          .map(
            (item) => `
              <tr>
                <td>
                  <strong>${escapeHTML(item.device_name || "-")}</strong><br />
                  <span class="mono">${escapeHTML(item.device_id || "-")}</span><br />
                  <span>${escapeHTML(item.device_type || "-")}</span>
                </td>
                <td>${statusPill(item.status)}</td>
                <td>
                  <div>${escapeHTML(item.lan_ip || "-")}</div>
                  <div class="mono">port: ${escapeHTML(item.port || 0)}</div>
                </td>
                <td>${escapeHTML(item.client_version || "-")}</td>
                <td>${escapeHTML(formatDate(item.last_seen_at))}</td>
              </tr>
            `
          )
          .join("")}
      </tbody>
    </table>
  `;
}

async function setupFilesPage() {
  const user = await ensureAuth();
  if (!user) {
    return;
  }

  const refreshIntervalMs = 5000;
  const uploadState = {
    selectedFile: null,
    activeTask: null,
  };

  renderShell("files", "文件页", "浏览文件资源，并通过浏览器端分片上传把文件送进云端资源池。");
  const page = document.getElementById("page-content");
  page.innerHTML = `
    <section class="page-header">
      <div>
        <h1 class="page-title">文件视图</h1>
        <p class="page-copy">上传过程会调用 <span class="mono">init-upload</span>、<span class="mono">missing-chunks</span>、分片上传和 <span class="mono">complete</span>，与 CLI 走同一条 V1 链路。</p>
      </div>
      <div class="toolbar">
        <span id="files-auto-status" class="user-chip">自动同步准备中</span>
        <button id="refresh-files" type="button">刷新文件列表</button>
      </div>
    </section>
    <div class="uploader">
      <p>选择一个文件后，浏览器会先计算整体 SHA-256，然后按 4MB 分片上传。</p>
      <div class="form-row">
        <input id="file-input" type="file" />
        <button id="upload-button" class="primary" type="button">开始上传</button>
      </div>
      <div id="upload-stage-grid" class="status-row" style="margin-top:12px">
        <div class="status-box">
          <div class="label">当前阶段</div>
          <div id="upload-stage-text">待命</div>
        </div>
        <div class="status-box">
          <div class="label">分片进度</div>
          <div id="upload-chunk-text">0 / 0</div>
        </div>
        <div class="status-box">
          <div class="label">已上传体积</div>
          <div id="upload-bytes-text">0 B / 0 B</div>
        </div>
      </div>
      <div class="progress" aria-hidden="true">
        <div id="upload-progress-bar" class="progress-bar"></div>
      </div>
      <div id="upload-progress-meta" class="progress-meta">还没有开始上传。</div>
    </div>
    <div id="resume-panel" style="margin-top:18px"></div>
    <div id="files-summary" style="margin-top:18px"></div>
    <div id="files-table" class="table-wrap"></div>
  `;

  function renderResumePanel() {
    const tasks = getUploadCache();
    const panel = document.getElementById("resume-panel");
    if (!tasks.length) {
      panel.innerHTML = "";
      return;
    }

    panel.innerHTML = `
      <div class="uploader">
        <p>待继续补传的浏览器任务保存在本地。重新选择同一个文件后，可以只补缺失分片。</p>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>文件</th>
                <th>任务</th>
                <th>状态</th>
                <th>本地进度</th>
                <th>最近更新时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              ${tasks
                .map(
                  (task) => `
                    <tr>
                      <td>
                        <strong>${escapeHTML(task.fileName || "-")}</strong><br />
                        <span>${escapeHTML(formatBytes(task.fileSize || 0))}</span>
                      </td>
                      <td>
                        <span class="mono">${escapeHTML(task.uploadId || "-")}</span><br />
                        <span class="mono">${escapeHTML(task.fileId || "-")}</span>
                      </td>
                      <td>
                        ${statusPill(task.status)}
                        <div>${escapeHTML(task.lastError || "-")}</div>
                      </td>
                      <td>${escapeHTML(`${task.completedChunks || 0} / ${task.totalChunks || 0}`)}</td>
                      <td>${escapeHTML(formatDate(task.updatedAt))}</td>
                      <td>
                        <button class="resume-upload-button" type="button" data-upload-id="${escapeHTML(task.uploadId)}">继续补传</button>
                        <button class="ghost clear-upload-button" type="button" data-upload-id="${escapeHTML(task.uploadId)}">清除</button>
                      </td>
                    </tr>
                  `
                )
                .join("")}
            </tbody>
          </table>
        </div>
      </div>
    `;

    panel.querySelectorAll(".resume-upload-button").forEach((button) => {
      button.addEventListener("click", async () => {
        const uploadId = button.getAttribute("data-upload-id");
        const task = getUploadTask(uploadId);
        if (!task) {
          setMessage("本地补传任务不存在。", "error");
          return;
        }

        const currentFile = uploadState.selectedFile;
        if (!currentFile) {
          setMessage("先重新选择对应的本地文件，再继续补传。", "error");
          return;
        }

        try {
          await resumeUploadFromBrowser(user, currentFile, task, uploadState);
          await refreshFiles("补传完成，正在刷新文件列表...");
        } catch (error) {
          setMessage(error.message, "error");
        }
      });
    });

    panel.querySelectorAll(".clear-upload-button").forEach((button) => {
      button.addEventListener("click", () => {
        removeUploadCache(button.getAttribute("data-upload-id"));
        renderResumePanel();
        setMessage("本地补传记录已清除。", "success");
      });
    });
  }

  async function refreshFiles(message) {
    try {
      if (message) {
        setMessage(message, "info");
      }
      const { body } = await apiFetch("/api/files");
      const items = body.items || [];
      const available = items.filter((item) => String(item.status).toLowerCase() === "available").length;
      document.getElementById("files-summary").innerHTML = renderSummaryCards([
        { label: "总文件数", value: String(items.length), note: "当前账号下未删除的文件记录" },
        { label: "可下载", value: String(available), note: "已经完成合并并通过 hash 校验" },
        {
          label: "累计体积",
          value: formatBytes(items.reduce((sum, item) => sum + Number(item.file_size || 0), 0)),
          note: "按文件元数据中的 file_size 汇总",
        },
      ]);
      document.getElementById("files-table").innerHTML = renderFilesTable(items);
      bindDownloadButtons();
      bindDeleteButtons();
      setAutoRefreshStatus("files-auto-status", refreshIntervalMs, new Date());
      if (message) {
        setMessage(`文件列表已刷新，当前共有 ${items.length} 个文件。`, "success");
      }
    } catch (error) {
      setMessage(error.message, "error");
    }
  }

  async function syncUploadTasks(trigger = "auto") {
    const tasks = getUploadCache();
    const liveTasks = tasks.filter(isLiveUploadTask);
    setAutoRefreshStatus("files-auto-status", refreshIntervalMs, new Date());

    if (!liveTasks.length) {
      if (trigger === "initial") {
        await refreshFiles("正在刷新文件列表...");
      }
      return;
    }

    let changed = false;
    let completedAny = false;

    for (const task of liveTasks) {
      try {
        const { body } = await apiFetch(`/api/uploads/${task.uploadId}/missing-chunks`);
        const nextMissing = body.missing_chunks || [];
        const nextUploaded = body.uploaded_chunks || [];
        const nextStatus =
          nextMissing.length === 0 && nextUploaded.length === (task.totalChunks || body.total_chunks || 0)
            ? "completed"
            : body.status || task.status;

        const nextTask = {
          ...task,
          totalChunks: body.total_chunks || task.totalChunks,
          completedChunks: nextUploaded.length,
          uploadedChunkIndexes: nextUploaded,
          missingChunks: nextMissing,
          status: nextStatus,
          lastError: nextStatus === "failed" ? task.lastError || "服务端任务失败" : "",
          updatedAt: new Date().toISOString(),
        };

        if (
          nextTask.completedChunks !== task.completedChunks ||
          nextTask.status !== task.status ||
          JSON.stringify(nextTask.missingChunks) !== JSON.stringify(task.missingChunks || [])
        ) {
          changed = true;
        }

        if (nextTask.status === "completed") {
          completedAny = true;
          removeUploadCache(nextTask.uploadId);
          if (uploadState.activeTask && uploadState.activeTask.uploadId === nextTask.uploadId) {
            uploadState.activeTask = null;
          }
        } else {
          upsertUploadCache(nextTask);
          if (uploadState.activeTask && uploadState.activeTask.uploadId === nextTask.uploadId) {
            uploadState.activeTask = nextTask;
            if (!uploadState.selectedFile) {
              updateProgress({
                percent: 8 + Math.round((nextTask.completedChunks / Math.max(nextTask.totalChunks || 1, 1)) * 82),
                stage: nextTask.status === "failed" ? "上传中断" : "服务端同步中",
                meta:
                  nextTask.status === "failed"
                    ? `服务端任务当前状态为失败。重新选择同一个文件后，可以继续补传。`
                    : `正在同步服务端状态，已上传 ${nextTask.completedChunks} / ${nextTask.totalChunks} 个分片。`,
                completedChunks: nextTask.completedChunks,
                totalChunks: nextTask.totalChunks,
                uploadedBytes: estimateUploadedBytes(nextTask),
                totalBytes: nextTask.fileSize,
              });
            }
          }
        }
      } catch (error) {
        const failedTask = {
          ...task,
          status: "failed",
          lastError: error.message,
          updatedAt: new Date().toISOString(),
        };
        upsertUploadCache(failedTask);
        if (uploadState.activeTask && uploadState.activeTask.uploadId === failedTask.uploadId) {
          uploadState.activeTask = failedTask;
        }
      }
    }

    renderResumePanel();

    if (completedAny) {
      await refreshFiles("检测到上传任务已完成，正在刷新文件列表...");
      return;
    }

    if (changed && trigger === "manual") {
      setMessage("已同步服务端上传状态。", "success");
    }
  }

  document.getElementById("refresh-files").addEventListener("click", () => refreshFiles("正在刷新文件列表..."));
  document.getElementById("file-input").addEventListener("change", (event) => {
    uploadState.selectedFile = event.target.files && event.target.files[0] ? event.target.files[0] : null;
    if (uploadState.selectedFile) {
      setMessage(`已选择文件 ${uploadState.selectedFile.name}，可以开始上传或继续补传。`, "info");
    }
  });
  document.getElementById("upload-button").addEventListener("click", async () => {
    const file = uploadState.selectedFile;
    if (!file) {
      setMessage("先选择一个要上传的文件。", "error");
      return;
    }

    try {
      await uploadFileFromBrowser(user, file, uploadState, renderResumePanel);
      document.getElementById("file-input").value = "";
      uploadState.selectedFile = null;
      await refreshFiles("上传完成，正在刷新文件列表...");
    } catch (error) {
      setMessage(error.message, "error");
    }
  });

  renderResumePanel();
  await refreshFiles("正在刷新文件列表...");
  await syncUploadTasks("initial");
  setupAutoRefresh(syncUploadTasks, refreshIntervalMs);
}

function renderFilesTable(items) {
  if (!items.length) {
    return `<div class="empty-state">当前还没有文件。可以先用 CLI 上传，或者直接在这个页面里选择文件上传。</div>`;
  }
  return `
    <table>
      <thead>
        <tr>
          <th>文件</th>
          <th>大小</th>
          <th>上传设备</th>
          <th>状态</th>
          <th>创建时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        ${items
          .map(
            (item) => `
              <tr>
                <td>
                  <strong>${escapeHTML(item.file_name || "-")}</strong><br />
                  <span class="mono">${escapeHTML(item.file_id || "-")}</span>
                </td>
                <td>${escapeHTML(formatBytes(Number(item.file_size || 0)))}</td>
                <td>${escapeHTML(item.uploader_device_id || "-")}</td>
                <td>${statusPill(item.status)}</td>
                <td>${escapeHTML(formatDate(item.created_at))}</td>
                <td>
                  <button class="download-button" type="button" data-file-id="${escapeHTML(item.file_id)}" data-file-name="${escapeHTML(item.file_name)}" ${
                    String(item.status).toLowerCase() !== "available" ? "disabled" : ""
                  }>
                    下载
                  </button>
                  <button class="ghost delete-button" type="button" data-file-id="${escapeHTML(item.file_id)}" data-file-name="${escapeHTML(item.file_name)}">
                    删除
                  </button>
                </td>
              </tr>
            `
          )
          .join("")}
      </tbody>
    </table>
  `;
}

function bindDownloadButtons() {
  document.querySelectorAll(".download-button").forEach((button) => {
    button.addEventListener("click", async () => {
      const fileId = button.getAttribute("data-file-id");
      const fileName = button.getAttribute("data-file-name") || "download.bin";
      try {
        setMessage(`正在下载 ${fileName}...`, "info");
        const result = await apiFetch(`/api/files/${fileId}/download`, { expectBlob: true });
        const blob = result.body;
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = fileName;
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.URL.revokeObjectURL(url);
        setMessage(`文件 ${fileName} 已开始下载。`, "success");
      } catch (error) {
        setMessage(error.message, "error");
      }
    });
  });
}

function bindDeleteButtons() {
  document.querySelectorAll(".delete-button").forEach((button) => {
    button.addEventListener("click", async () => {
      const fileId = button.getAttribute("data-file-id");
      const fileName = button.getAttribute("data-file-name") || fileId || "unknown";
      if (!fileId) {
        setMessage("缺少 file_id，无法删除。", "error");
        return;
      }

      const confirmed = window.confirm(`确认删除文件“${fileName}”吗？删除后将从服务器移除，无法继续下载。`);
      if (!confirmed) {
        return;
      }

      try {
        button.disabled = true;
        setMessage(`正在删除 ${fileName}...`, "info");
        await apiFetch(`/api/files/${fileId}`, { method: "DELETE" });
        removeUploadCacheByFileId(fileId);
        setMessage(`文件 ${fileName} 已删除。`, "success");
        await refreshFiles();
      } catch (error) {
        setMessage(error.message, "error");
      } finally {
        button.disabled = false;
      }
    });
  });
}

async function uploadFileFromBrowser(user, file, uploadState, renderResumePanel) {
  const chunkSize = 4 * 1024 * 1024;
  const totalChunks = Math.ceil(file.size / chunkSize) || 1;
  updateProgress({
    percent: 2,
    stage: "计算文件摘要",
    meta: "正在计算文件 SHA-256...",
    completedChunks: 0,
    totalChunks,
    uploadedBytes: 0,
    totalBytes: file.size,
  });
  const fileHash = await hashBlob(file);
  const deviceId = user.username ? `${user.username}-web` : "web-ui";

  updateProgress({
    percent: 6,
    stage: "初始化上传任务",
    meta: "正在向服务端申请 upload_id 和 file_id...",
    completedChunks: 0,
    totalChunks,
    uploadedBytes: 0,
    totalBytes: file.size,
  });
  const initResult = await apiFetch("/api/files/init-upload", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      device_id: deviceId,
      file_name: file.name,
      file_size: file.size,
      file_hash: fileHash,
      chunk_size: chunkSize,
      total_chunks: totalChunks,
    }),
  }).catch(async (error) => {
    if (!error.message.includes("DEVICE_NOT_FOUND")) {
      throw error;
    }
    await registerWebDevice(deviceId);
    return apiFetch("/api/files/init-upload", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        device_id: deviceId,
        file_name: file.name,
        file_size: file.size,
        file_hash: fileHash,
        chunk_size: chunkSize,
        total_chunks: totalChunks,
      }),
    });
  });

  const upload = initResult.body;
  if (upload.status === "available") {
    updateProgress({
      percent: 100,
      stage: "秒传完成",
      meta: "服务端已存在同 hash 文件，直接复用已有资源。",
      completedChunks: totalChunks,
      totalChunks,
      uploadedBytes: file.size,
      totalBytes: file.size,
    });
    return;
  }

  const task = {
    uploadId: upload.upload_id,
    fileId: upload.file_id,
    fileName: file.name,
    fileSize: file.size,
    fileHash,
    chunkSize,
    totalChunks,
    completedChunks: (upload.uploaded_chunks || []).length,
    uploadedChunkIndexes: upload.uploaded_chunks || [],
    missingChunks: upload.missing_chunks || [],
    status: upload.status || "initialized",
    lastError: "",
    updatedAt: new Date().toISOString(),
    deviceId,
  };
  uploadState.activeTask = task;
  upsertUploadCache(task);
  renderResumePanel();

  await continueUploadTask(user, file, task, uploadState, renderResumePanel);
}

async function registerWebDevice(deviceId) {
  await apiFetch("/api/devices/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      device_id: deviceId,
      device_name: "Web UI",
      device_type: "web",
      public_key: "",
      client_version: "0.1.0-ui",
    }),
  });
}

function updateProgress(state) {
  const bar = document.getElementById("upload-progress-bar");
  const meta = document.getElementById("upload-progress-meta");
  const stage = document.getElementById("upload-stage-text");
  const chunkText = document.getElementById("upload-chunk-text");
  const bytesText = document.getElementById("upload-bytes-text");

  if (bar) {
    bar.style.width = formatPercent(state.percent);
  }
  if (meta) {
    meta.textContent = state.meta;
  }
  if (stage) {
    stage.textContent = state.stage;
  }
  if (chunkText) {
    chunkText.textContent = `${state.completedChunks || 0} / ${state.totalChunks || 0}`;
  }
  if (bytesText) {
    bytesText.textContent = `${formatBytes(state.uploadedBytes || 0)} / ${formatBytes(state.totalBytes || 0)}`;
  }
}

async function resumeUploadFromBrowser(user, file, cachedTask, uploadState, renderResumePanel) {
  updateProgress({
    percent: 3,
    stage: "校验本地文件",
    meta: "正在确认重新选择的文件是否与补传任务一致...",
    completedChunks: cachedTask.completedChunks || 0,
    totalChunks: cachedTask.totalChunks || 0,
    uploadedBytes: Math.min((cachedTask.completedChunks || 0) * (cachedTask.chunkSize || 0), cachedTask.fileSize || 0),
    totalBytes: cachedTask.fileSize || 0,
  });

  if (file.size !== cachedTask.fileSize) {
    throw new Error("重新选择的文件大小不一致，不能继续补传。");
  }

  const fileHash = await hashBlob(file);
  if (fileHash !== cachedTask.fileHash) {
    throw new Error("重新选择的文件 hash 不一致，不能继续补传。");
  }

  const { body } = await apiFetch(`/api/uploads/${cachedTask.uploadId}/missing-chunks`);
  cachedTask.missingChunks = body.missing_chunks || [];
  cachedTask.uploadedChunkIndexes = body.uploaded_chunks || [];
  cachedTask.completedChunks = cachedTask.uploadedChunkIndexes.length;
  cachedTask.status = body.status || cachedTask.status;
  cachedTask.lastError = "";
  cachedTask.updatedAt = new Date().toISOString();
  uploadState.activeTask = cachedTask;
  upsertUploadCache(cachedTask);
  renderResumePanel();

  await continueUploadTask(user, file, cachedTask, uploadState, renderResumePanel);
}

async function continueUploadTask(user, file, task, uploadState, renderResumePanel) {
  const missingChunks = [...(task.missingChunks || [])];
  let completed = task.completedChunks || 0;
  let uploadedBytes = estimateUploadedBytes(task);

  try {
    for (const [position, chunkIndex] of missingChunks.entries()) {
      const start = chunkIndex * task.chunkSize;
      const end = Math.min(file.size, start + task.chunkSize);
      const chunk = file.slice(start, end);
      const chunkHash = await hashBlob(chunk);
      updateProgress({
        percent: 8 + Math.round((completed / task.totalChunks) * 82),
        stage: "上传分片",
        meta: `正在上传第 ${chunkIndex + 1} / ${task.totalChunks} 个分片，还剩 ${missingChunks.length - position} 个缺失分片。`,
        completedChunks: completed,
        totalChunks: task.totalChunks,
        uploadedBytes,
        totalBytes: file.size,
      });

      await apiFetch(`/api/uploads/${task.uploadId}/chunks/${chunkIndex}`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/octet-stream",
          "X-Chunk-Hash": chunkHash,
        },
        body: chunk,
      });

      completed += 1;
      uploadedBytes += chunk.size;
      task.uploadedChunkIndexes = appendUniqueNumber(task.uploadedChunkIndexes || [], chunkIndex);
      task.completedChunks = completed;
      task.missingChunks = missingChunks.slice(position + 1);
      task.status = "uploading";
      task.lastError = "";
      task.updatedAt = new Date().toISOString();
      uploadState.activeTask = task;
      upsertUploadCache(task);
      renderResumePanel();

      updateProgress({
        percent: 8 + Math.round((completed / task.totalChunks) * 82),
        stage: "上传分片",
        meta: `分片上传进度 ${completed} / ${task.totalChunks}，当前已补传到第 ${chunkIndex + 1} 个分片。`,
        completedChunks: completed,
        totalChunks: task.totalChunks,
        uploadedBytes,
        totalBytes: file.size,
      });
    }

    updateProgress({
      percent: 94,
      stage: "服务端合并",
      meta: "全部分片已上传，正在请求服务端合并并校验整体 hash...",
      completedChunks: task.totalChunks,
      totalChunks: task.totalChunks,
      uploadedBytes: file.size,
      totalBytes: file.size,
    });

    const completeResult = await apiFetch(`/api/uploads/${task.uploadId}/complete`, {
      method: "POST",
    });
    if (completeResult.body && completeResult.body.missing_chunks && completeResult.body.missing_chunks.length) {
      task.status = "failed";
      task.lastError = `complete 阶段仍缺少分片: ${completeResult.body.missing_chunks.join(", ")}`;
      task.missingChunks = completeResult.body.missing_chunks;
      task.updatedAt = new Date().toISOString();
      upsertUploadCache(task);
      renderResumePanel();
      throw new Error(task.lastError);
    }

    task.status = "completed";
    task.completedChunks = task.totalChunks;
    task.missingChunks = [];
    task.lastError = "";
    task.updatedAt = new Date().toISOString();
    removeUploadCache(task.uploadId);
    renderResumePanel();

    updateProgress({
      percent: 100,
      stage: "上传完成",
      meta: `上传完成，file_id=${completeResult.body.file_id}`,
      completedChunks: task.totalChunks,
      totalChunks: task.totalChunks,
      uploadedBytes: file.size,
      totalBytes: file.size,
    });
    setMessage(`文件 ${file.name} 上传完成。`, "success");
  } catch (error) {
    task.status = "failed";
    task.lastError = error.message;
    task.updatedAt = new Date().toISOString();
    upsertUploadCache(task);
    renderResumePanel();
    updateProgress({
      percent: 8 + Math.round(((task.completedChunks || 0) / task.totalChunks) * 82),
      stage: "上传中断",
      meta: `上传已中断：${error.message}。重新选择同一个文件后，可以在下方继续补传。`,
      completedChunks: task.completedChunks || 0,
      totalChunks: task.totalChunks,
      uploadedBytes: estimateUploadedBytes(task),
      totalBytes: file.size,
    });
    throw error;
  }
}

function estimateUploadedBytes(task) {
  const chunkSize = task.chunkSize || 0;
  const totalBytes = task.fileSize || 0;
  if (!task.uploadedChunkIndexes || !task.uploadedChunkIndexes.length) {
    return 0;
  }

  const fullChunks = Math.max(0, task.uploadedChunkIndexes.length - 1);
  let estimated = fullChunks * chunkSize;
  const lastChunkIndex = task.totalChunks - 1;
  if (task.uploadedChunkIndexes.indexOf(lastChunkIndex) >= 0) {
    const lastChunkBytes = totalBytes - lastChunkIndex * chunkSize;
    estimated = Math.max(estimated, (task.uploadedChunkIndexes.length - 1) * chunkSize + Math.max(lastChunkBytes, 0));
  } else {
    estimated = task.uploadedChunkIndexes.length * chunkSize;
  }
  return Math.min(estimated, totalBytes);
}

function appendUniqueNumber(list, value) {
  const next = Array.isArray(list) ? list.slice() : [];
  if (next.indexOf(value) === -1) {
    next.push(value);
    next.sort((left, right) => left - right);
  }
  return next;
}

async function hashBlob(blob) {
  const buffer = await blob.arrayBuffer();
  if (window.crypto && window.crypto.subtle && typeof window.crypto.subtle.digest === "function") {
    const digest = await window.crypto.subtle.digest("SHA-256", buffer);
    return bytesToHex(new Uint8Array(digest));
  }
  return sha256Fallback(new Uint8Array(buffer));
}

function bytesToHex(bytes) {
  return [...bytes].map((value) => value.toString(16).padStart(2, "0")).join("");
}

function sha256Fallback(bytes) {
  const initialHash = [
    0x6a09e667,
    0xbb67ae85,
    0x3c6ef372,
    0xa54ff53a,
    0x510e527f,
    0x9b05688c,
    0x1f83d9ab,
    0x5be0cd19,
  ];

  const roundConstants = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
  ];

  const paddedLength = Math.ceil((bytes.length + 9) / 64) * 64;
  const padded = new Uint8Array(paddedLength);
  padded.set(bytes);
  padded[bytes.length] = 0x80;

  const bitLength = BigInt(bytes.length) * 8n;
  for (let index = 0; index < 8; index += 1) {
    padded[padded.length - 1 - index] = Number((bitLength >> BigInt(index * 8)) & 0xffn);
  }

  const hash = initialHash.slice();
  const schedule = new Uint32Array(64);

  for (let offset = 0; offset < padded.length; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      const base = offset + index * 4;
      schedule[index] =
        ((padded[base] << 24) | (padded[base + 1] << 16) | (padded[base + 2] << 8) | padded[base + 3]) >>> 0;
    }

    for (let index = 16; index < 64; index += 1) {
      const s0 =
        (rightRotate(schedule[index - 15], 7) ^
          rightRotate(schedule[index - 15], 18) ^
          (schedule[index - 15] >>> 3)) >>>
        0;
      const s1 =
        (rightRotate(schedule[index - 2], 17) ^
          rightRotate(schedule[index - 2], 19) ^
          (schedule[index - 2] >>> 10)) >>>
        0;
      schedule[index] = (schedule[index - 16] + s0 + schedule[index - 7] + s1) >>> 0;
    }

    let [a, b, c, d, e, f, g, h] = hash;
    for (let index = 0; index < 64; index += 1) {
      const sigma1 = (rightRotate(e, 6) ^ rightRotate(e, 11) ^ rightRotate(e, 25)) >>> 0;
      const choice = ((e & f) ^ (~e & g)) >>> 0;
      const temp1 = (h + sigma1 + choice + roundConstants[index] + schedule[index]) >>> 0;
      const sigma0 = (rightRotate(a, 2) ^ rightRotate(a, 13) ^ rightRotate(a, 22)) >>> 0;
      const majority = ((a & b) ^ (a & c) ^ (b & c)) >>> 0;
      const temp2 = (sigma0 + majority) >>> 0;

      h = g;
      g = f;
      f = e;
      e = (d + temp1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (temp1 + temp2) >>> 0;
    }

    hash[0] = (hash[0] + a) >>> 0;
    hash[1] = (hash[1] + b) >>> 0;
    hash[2] = (hash[2] + c) >>> 0;
    hash[3] = (hash[3] + d) >>> 0;
    hash[4] = (hash[4] + e) >>> 0;
    hash[5] = (hash[5] + f) >>> 0;
    hash[6] = (hash[6] + g) >>> 0;
    hash[7] = (hash[7] + h) >>> 0;
  }

  const output = new Uint8Array(32);
  for (let index = 0; index < hash.length; index += 1) {
    output[index * 4] = (hash[index] >>> 24) & 0xff;
    output[index * 4 + 1] = (hash[index] >>> 16) & 0xff;
    output[index * 4 + 2] = (hash[index] >>> 8) & 0xff;
    output[index * 4 + 3] = hash[index] & 0xff;
  }
  return bytesToHex(output);
}

function rightRotate(value, shift) {
  return (value >>> shift) | (value << (32 - shift));
}

async function setupTasksPage() {
  const user = await ensureAuth();
  if (!user) {
    return;
  }

  const refreshIntervalMs = 5000;
  renderShell("tasks", "任务页", "查看上传任务状态、已上传分片数和失败原因。");
  const page = document.getElementById("page-content");
  page.innerHTML = `
    <section class="page-header">
      <div>
        <h1 class="page-title">任务视图</h1>
        <p class="page-copy">这里直接读取 <span class="mono">/api/tasks</span>。如果你正在上传大文件，刷新就能看到 upload_id 和分片进度变化。</p>
      </div>
      <div class="toolbar">
        <span id="tasks-auto-status" class="user-chip">自动轮询准备中</span>
        <button id="refresh-tasks" type="button">刷新任务列表</button>
      </div>
    </section>
    <div id="tasks-summary"></div>
    <div id="tasks-table" class="table-wrap"></div>
  `;

  const refresh = async (trigger = "manual") => {
    try {
      if (trigger === "manual" || trigger === "initial") {
        setMessage("正在刷新任务列表...", "info");
      }
      const { body } = await apiFetch("/api/tasks");
      const items = body.items || [];
      document.getElementById("tasks-summary").innerHTML = renderSummaryCards([
        { label: "总任务数", value: String(items.length), note: "当前账号下的上传任务记录" },
        {
          label: "进行中",
          value: String(items.filter((item) => ["initialized", "uploading"].includes(String(item.status).toLowerCase())).length),
          note: "尚未完成的任务",
        },
        {
          label: "已完成",
          value: String(items.filter((item) => String(item.status).toLowerCase() === "completed").length),
          note: "已经合并并校验通过",
        },
      ]);
      document.getElementById("tasks-table").innerHTML = renderTasksTable(items);
      setAutoRefreshStatus("tasks-auto-status", refreshIntervalMs, new Date());
      if (trigger === "manual" || trigger === "initial") {
        setMessage(`任务列表已刷新，当前共有 ${items.length} 条任务记录。`, "success");
      }
    } catch (error) {
      setMessage(error.message, "error");
    }
  };

  document.getElementById("refresh-tasks").addEventListener("click", () => refresh("manual"));
  await refresh("initial");
  setupAutoRefresh(refresh, refreshIntervalMs);
}

function renderTasksTable(items) {
  if (!items.length) {
    return `<div class="empty-state">当前还没有上传任务。去文件页上传一个文件后再来看这里。</div>`;
  }
  return `
    <table>
      <thead>
        <tr>
          <th>任务</th>
          <th>文件</th>
          <th>分片进度</th>
          <th>状态</th>
          <th>错误</th>
          <th>更新时间</th>
        </tr>
      </thead>
      <tbody>
        ${items
          .map(
            (item) => `
              <tr>
                <td><span class="mono">${escapeHTML(item.upload_id || "-")}</span></td>
                <td>
                  <strong>${escapeHTML(item.file_name || "-")}</strong><br />
                  <span class="mono">${escapeHTML(item.file_id || "-")}</span>
                </td>
                <td>${escapeHTML(`${item.uploaded_chunks || 0} / ${item.total_chunks || 0}`)}</td>
                <td>${statusPill(item.status)}</td>
                <td>${escapeHTML(item.error_message || "-")}</td>
                <td>${escapeHTML(formatDate(item.updated_at))}</td>
              </tr>
            `
          )
          .join("")}
      </tbody>
    </table>
  `;
}

document.addEventListener("DOMContentLoaded", async () => {
  const page = document.body.getAttribute("data-page");
  if (page === "login") {
    await setupLoginPage();
    return;
  }
  if (page === "devices") {
    await setupDevicesPage();
    return;
  }
  if (page === "files") {
    await setupFilesPage();
    return;
  }
  if (page === "tasks") {
    await setupTasksPage();
  }
});
