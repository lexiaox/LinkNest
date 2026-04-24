//go:build windows

package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"linknest/client/internal/appsvc"
	"linknest/client/internal/device"
	"linknest/client/internal/transfer"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const (
	deviceItemHeight = 94
	fileItemHeight   = 76
	taskItemHeight   = 94
)

type DesktopApp struct {
	svc    *appsvc.Service
	app    fyne.App
	window fyne.Window

	serverEntry   *widget.Entry
	usernameEntry *widget.Entry
	emailEntry    *widget.Entry
	passwordEntry *widget.Entry
	deviceName    *widget.Entry
	deviceType    *widget.Entry

	statusLabel      *widget.Label
	snapshotLabel    *widget.Label
	activityLabel    *widget.Label
	lastRefreshLabel *widget.Label
	busyBar          *widget.ProgressBarInfinite

	devices        []device.RemoteDevice
	selectedDevice int
	deviceList     *widget.List

	files        []transfer.RemoteFile
	selectedFile int
	fileList     *widget.List

	tasks                []transfer.RemoteTask
	selectedTask         int
	taskList             *widget.List
	selectedTaskLabel    *widget.Label
	selectedTaskProgress *widget.ProgressBar

	autoRefreshStopCh chan struct{}
}

func Launch(root string) error {
	svc, err := appsvc.New(root)
	if err != nil {
		return err
	}

	gui := &DesktopApp{
		svc:            svc,
		app:            app.NewWithID("top.ledouya.linknest.desktop"),
		selectedDevice: -1,
		selectedFile:   -1,
		selectedTask:   -1,
	}
	gui.window = gui.app.NewWindow("LinkNest Desktop")
	gui.window.Resize(fyne.NewSize(1240, 820))
	gui.window.SetContent(gui.buildContent())
	gui.window.SetCloseIntercept(func() {
		gui.stopAutoRefresh()
		gui.svc.StopHeartbeat()
		gui.window.Close()
	})
	gui.refreshSnapshot()
	gui.preloadDataIfReady()
	gui.startAutoRefresh()
	gui.window.ShowAndRun()
	return nil
}

func (d *DesktopApp) buildContent() fyne.CanvasObject {
	snapshot := d.svc.Snapshot()

	d.serverEntry = widget.NewEntry()
	d.serverEntry.SetText(snapshot.ServerURL)

	d.usernameEntry = widget.NewEntry()
	d.usernameEntry.SetPlaceHolder("用户名")

	d.emailEntry = widget.NewEntry()
	d.emailEntry.SetPlaceHolder("邮箱（注册时必填）")

	d.passwordEntry = widget.NewPasswordEntry()
	d.passwordEntry.SetPlaceHolder("密码")

	d.deviceName = widget.NewEntry()
	d.deviceName.SetPlaceHolder("设备名（默认主机名）")

	d.deviceType = widget.NewEntry()
	d.deviceType.SetPlaceHolder("设备类型（默认 windows）")

	d.statusLabel = widget.NewLabel("就绪。")
	d.statusLabel.Wrapping = fyne.TextWrapWord

	d.activityLabel = widget.NewLabel("后台当前没有正在执行的操作。")
	d.activityLabel.Wrapping = fyne.TextWrapWord

	d.lastRefreshLabel = widget.NewLabel("最近自动刷新：尚未开始")
	d.lastRefreshLabel.Wrapping = fyne.TextWrapWord

	d.busyBar = widget.NewProgressBarInfinite()
	d.busyBar.Hide()

	d.snapshotLabel = widget.NewLabel("")
	d.snapshotLabel.Wrapping = fyne.TextWrapWord

	accountTab := container.NewTabItem("账号", d.buildAccountTab())
	deviceTab := container.NewTabItem("设备", d.buildDeviceTab())
	fileTab := container.NewTabItem("文件", d.buildFileTab())
	taskTab := container.NewTabItem("上传任务", d.buildTaskTab())

	tabs := container.NewAppTabs(accountTab, deviceTab, fileTab, taskTab)
	tabs.SetTabLocation(container.TabLocationTop)

	header := container.NewVBox(
		widget.NewLabelWithStyle("LinkNest Windows GUI", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("桌面端直接复用现有 Go 客户端模块，不再通过 CLI 子进程调用。"),
		d.snapshotLabel,
	)

	footer := container.NewVBox(
		widget.NewSeparator(),
		d.activityLabel,
		d.busyBar,
		d.lastRefreshLabel,
		d.statusLabel,
	)

	return container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		footer,
		nil,
		nil,
		tabs,
	)
}

func (d *DesktopApp) buildAccountTab() fyne.CanvasObject {
	saveServerButton := widget.NewButton("保存服务器地址", func() {
		if err := d.svc.SetServerURL(d.serverEntry.Text); err != nil {
			d.showError(err)
			return
		}
		d.setStatus("服务器地址已保存。")
		d.refreshSnapshot()
	})

	loginButton := widget.NewButton("登录", func() {
		var username string
		d.runAsync("正在登录...", func() error {
			result, err := d.svc.Login(d.usernameEntry.Text, d.passwordEntry.Text)
			if err != nil {
				return err
			}
			username = result.User.Username
			return nil
		}, func() {
			d.setStatus(fmt.Sprintf("登录成功，当前用户：%s", username))
			d.preloadDataIfReady()
		})
	})

	registerButton := widget.NewButton("注册", func() {
		var message string
		d.runAsync("正在注册账号...", func() error {
			result, err := d.svc.Register(d.usernameEntry.Text, d.emailEntry.Text, d.passwordEntry.Text)
			if err != nil {
				return err
			}
			message = fmt.Sprintf("注册成功，当前用户：%s", result.User.Username)
			if strings.TrimSpace(result.Notice) != "" {
				message += "；" + strings.TrimSpace(result.Notice)
			}
			return nil
		}, func() {
			d.setStatus(message)
			d.preloadDataIfReady()
		})
	})

	deleteButton := widget.NewButton("注销账号", func() {
		dialog.ShowConfirm("注销账号", "确认注销当前账号吗？这会删除该账号下的设备、文件和上传任务。", func(ok bool) {
			if !ok {
				return
			}

			var deletedUser string
			d.runAsync("正在注销账号并清理服务器数据...", func() error {
				result, err := d.svc.DeleteAccount(d.passwordEntry.Text)
				if err != nil {
					return err
				}
				deletedUser = result.User.Username
				return nil
			}, func() {
				d.devices = nil
				d.files = nil
				d.tasks = nil
				d.selectedDevice = -1
				d.selectedFile = -1
				d.selectedTask = -1
				d.deviceList.Refresh()
				d.fileList.Refresh()
				d.taskList.Refresh()
				d.updateSelectedTaskSummary()
				d.setStatus(fmt.Sprintf("账号 %s 已注销，服务器数据已清理。", deletedUser))
			})
		}, d.window)
	})

	return container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("服务器地址", d.serverEntry),
			widget.NewFormItem("用户名", d.usernameEntry),
			widget.NewFormItem("邮箱", d.emailEntry),
			widget.NewFormItem("密码", d.passwordEntry),
		),
		container.NewGridWithColumns(4, saveServerButton, loginButton, registerButton, deleteButton),
		widget.NewCard("说明", "", widget.NewLabel("推荐流程：先保存服务器地址，再登录或注册；如果你要正式绑定这台电脑，请到“设备”页点击“绑定当前设备”。")),
	)
}

func (d *DesktopApp) buildDeviceTab() fyne.CanvasObject {
	d.deviceList = widget.NewList(
		func() int { return len(d.devices) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatDeviceItem(d.devices[id]))
		},
	)
	d.deviceList.OnSelected = func(id widget.ListItemID) {
		d.selectedDevice = id
	}

	bindButton := widget.NewButton("绑定当前设备", func() {
		var profile device.Profile
		d.runAsync("正在绑定当前设备...", func() error {
			var err error
			profile, err = d.svc.BindCurrentDevice(d.deviceName.Text, d.deviceType.Text)
			return err
		}, func() {
			d.setStatus(fmt.Sprintf("设备已绑定：%s (%s)", profile.DeviceName, profile.DeviceID))
			d.refreshDevices(false)
		})
	})

	refreshButton := widget.NewButton("刷新设备列表", func() {
		d.runAsync("正在刷新设备列表...", func() error {
			return d.refreshDevices(true)
		}, func() {
			d.setStatus(fmt.Sprintf("设备列表已刷新，共 %d 台设备。", len(d.devices)))
		})
	})

	startHeartbeatButton := widget.NewButton("开始在线心跳", func() {
		if err := d.svc.StartHeartbeat(); err != nil {
			d.showError(err)
			return
		}
		d.setStatus("在线心跳已启动。")
		d.refreshSnapshot()
	})

	stopHeartbeatButton := widget.NewButton("停止在线心跳", func() {
		d.svc.StopHeartbeat()
		d.setStatus("在线心跳已停止。")
		d.refreshSnapshot()
	})

	return container.NewBorder(
		container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("设备名", d.deviceName),
				widget.NewFormItem("设备类型", d.deviceType),
			),
			container.NewGridWithColumns(4, bindButton, refreshButton, startHeartbeatButton, stopHeartbeatButton),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		d.deviceList,
	)
}

func (d *DesktopApp) buildFileTab() fyne.CanvasObject {
	d.fileList = widget.NewList(
		func() int { return len(d.files) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatFileItem(d.files[id]))
		},
	)
	d.fileList.OnSelected = func(id widget.ListItemID) {
		d.selectedFile = id
	}

	refreshButton := widget.NewButton("刷新文件列表", func() {
		d.runAsync("正在刷新文件列表...", func() error {
			return d.refreshFiles(true)
		}, func() {
			d.setStatus(fmt.Sprintf("文件列表已刷新，共 %d 个文件。", len(d.files)))
		})
	})

	uploadButton := widget.NewButton("上传文件", func() {
		open := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				d.showError(err)
				return
			}
			if reader == nil {
				return
			}

			path := reader.URI().Path()
			_ = reader.Close()

			d.runAsync("正在上传文件...", func() error {
				if err := d.svc.Upload(path); err != nil {
					return err
				}
				if err := d.refreshFiles(true); err != nil {
					return err
				}
				return d.refreshTasks(true)
			}, func() {
				d.setStatus(fmt.Sprintf("上传完成：%s", path))
			})
		}, d.window)
		open.Show()
	})

	downloadButton := widget.NewButton("下载选中文件", func() {
		file, err := d.selectedRemoteFile()
		if err != nil {
			d.showError(err)
			return
		}
		save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, saveErr error) {
			if saveErr != nil {
				d.showError(saveErr)
				return
			}
			if writer == nil {
				return
			}

			path := writer.URI().Path()
			_ = writer.Close()

			d.runAsync("正在下载文件...", func() error {
				return d.svc.Download(file.FileID, path)
			}, func() {
				d.setStatus(fmt.Sprintf("下载完成：%s -> %s", file.FileName, path))
			})
		}, d.window)
		save.SetFileName(file.FileName)
		save.Show()
	})

	deleteButton := widget.NewButton("删除选中文件", func() {
		file, err := d.selectedRemoteFile()
		if err != nil {
			d.showError(err)
			return
		}
		dialog.ShowConfirm("删除文件", fmt.Sprintf("确认删除文件 %s 吗？", file.FileName), func(ok bool) {
			if !ok {
				return
			}

			d.runAsync("正在删除文件...", func() error {
				if err := d.svc.DeleteFile(file.FileID); err != nil {
					return err
				}
				if err := d.refreshFiles(true); err != nil {
					return err
				}
				return d.refreshTasks(true)
			}, func() {
				d.setStatus(fmt.Sprintf("文件已删除：%s", file.FileName))
			})
		}, d.window)
	})

	return container.NewBorder(
		container.NewVBox(
			container.NewGridWithColumns(4, refreshButton, uploadButton, downloadButton, deleteButton),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		d.fileList,
	)
}

func (d *DesktopApp) buildTaskTab() fyne.CanvasObject {
	d.taskList = widget.NewList(
		func() int { return len(d.tasks) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatTaskItem(d.tasks[id]))
		},
	)
	d.taskList.OnSelected = func(id widget.ListItemID) {
		d.selectedTask = id
		d.updateSelectedTaskSummary()
	}

	d.selectedTaskLabel = widget.NewLabel("请选择一条上传任务查看详情。这里显示的是每次文件上传或续传的记录。")
	d.selectedTaskLabel.Wrapping = fyne.TextWrapWord

	d.selectedTaskProgress = widget.NewProgressBar()
	d.selectedTaskProgress.Min = 0
	d.selectedTaskProgress.Max = 1
	d.selectedTaskProgress.SetValue(0)
	d.selectedTaskProgress.Hide()

	refreshButton := widget.NewButton("刷新任务列表", func() {
		d.runAsync("正在刷新任务列表...", func() error {
			return d.refreshTasks(true)
		}, func() {
			d.setStatus(fmt.Sprintf("上传任务列表已刷新，共 %d 条任务。", len(d.tasks)))
		})
	})

	resumeButton := widget.NewButton("继续选中任务", func() {
		task, err := d.selectedRemoteTask()
		if err != nil {
			d.showError(err)
			return
		}

		d.runAsync("正在继续上传任务...", func() error {
			if err := d.svc.ResumeTask(task.UploadID); err != nil {
				return err
			}
			if err := d.refreshTasks(true); err != nil {
				return err
			}
			return d.refreshFiles(true)
		}, func() {
			d.setStatus(fmt.Sprintf("上传任务已继续：%s", task.UploadID))
		})
	})

	return container.NewBorder(
		container.NewVBox(
			d.selectedTaskLabel,
			d.selectedTaskProgress,
			container.NewGridWithColumns(2, refreshButton, resumeButton),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		d.taskList,
	)
}

func (d *DesktopApp) preloadDataIfReady() {
	snapshot := d.svc.Snapshot()
	if !snapshot.HasToken {
		return
	}
	_ = d.refreshDevices(true)
	_ = d.refreshFiles(true)
	_ = d.refreshTasks(true)
}

func (d *DesktopApp) startAutoRefresh() {
	if d.autoRefreshStopCh != nil {
		return
	}

	stopCh := make(chan struct{})
	d.autoRefreshStopCh = stopCh

	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()

		cycle := 0
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
			}

			snapshot := d.svc.Snapshot()
			if !snapshot.HasToken {
				continue
			}

			cycle++

			devices, devErr := d.svc.ListDevices()
			tasks, taskErr := d.svc.ListTasks()
			var files []transfer.RemoteFile
			var fileErr error
			if cycle%2 == 0 {
				files, fileErr = d.svc.ListFiles()
			}

			now := time.Now()
			fyne.Do(func() {
				if devErr == nil {
					d.devices = devices
					d.deviceList.Refresh()
					applyListHeight(d.deviceList, len(d.devices), deviceItemHeight)
				}
				if taskErr == nil {
					d.tasks = tasks
					d.taskList.Refresh()
					applyListHeight(d.taskList, len(d.tasks), taskItemHeight)
					d.updateSelectedTaskSummary()
				}
				if fileErr == nil && files != nil {
					d.files = files
					d.fileList.Refresh()
					applyListHeight(d.fileList, len(d.files), fileItemHeight)
				}
				d.lastRefreshLabel.SetText("最近自动刷新：" + now.Format("2006-01-02 15:04:05"))
				d.refreshSnapshot()
			})
		}
	}()
}

func (d *DesktopApp) stopAutoRefresh() {
	if d.autoRefreshStopCh == nil {
		return
	}
	close(d.autoRefreshStopCh)
	d.autoRefreshStopCh = nil
}

func (d *DesktopApp) refreshDevices(silent bool) error {
	items, err := d.svc.ListDevices()
	if err != nil {
		if !silent {
			d.showError(err)
		}
		return err
	}
	d.devices = items
	d.selectedDevice = -1
	d.deviceList.Refresh()
	applyListHeight(d.deviceList, len(d.devices), deviceItemHeight)
	d.markRefreshed()
	return nil
}

func (d *DesktopApp) refreshFiles(silent bool) error {
	items, err := d.svc.ListFiles()
	if err != nil {
		if !silent {
			d.showError(err)
		}
		return err
	}
	d.files = items
	d.selectedFile = -1
	d.fileList.Refresh()
	applyListHeight(d.fileList, len(d.files), fileItemHeight)
	d.markRefreshed()
	return nil
}

func (d *DesktopApp) refreshTasks(silent bool) error {
	items, err := d.svc.ListTasks()
	if err != nil {
		if !silent {
			d.showError(err)
		}
		return err
	}
	d.tasks = items
	d.selectedTask = -1
	d.taskList.Refresh()
	applyListHeight(d.taskList, len(d.tasks), taskItemHeight)
	d.updateSelectedTaskSummary()
	d.markRefreshed()
	return nil
}

func (d *DesktopApp) refreshSnapshot() {
	snapshot := d.svc.Snapshot()

	tokenText := "未登录"
	if snapshot.HasToken {
		tokenText = "已登录"
	}

	deviceText := "未绑定"
	if strings.TrimSpace(snapshot.DeviceID) != "" {
		deviceText = fmt.Sprintf("%s (%s)", snapshot.DeviceName, snapshot.DeviceID)
	}

	heartbeatText := "未运行"
	if snapshot.HeartbeatRunning {
		heartbeatText = "运行中"
	}
	if strings.TrimSpace(snapshot.HeartbeatError) != "" {
		heartbeatText += "；最近错误：" + strings.TrimSpace(snapshot.HeartbeatError)
	}

	d.snapshotLabel.SetText(fmt.Sprintf(
		"服务器：%s\n登录状态：%s\n当前设备：%s\n在线心跳：%s\n本地配置目录：%s",
		emptyAs(snapshot.ServerURL, "未设置"),
		tokenText,
		deviceText,
		heartbeatText,
		d.svc.Root(),
	))
}

func (d *DesktopApp) updateSelectedTaskSummary() {
	if d.selectedTask < 0 || d.selectedTask >= len(d.tasks) {
		d.selectedTaskLabel.SetText("请选择一条上传任务查看详情。自动刷新会保持上传进度为最新状态。")
		d.selectedTaskProgress.SetValue(0)
		d.selectedTaskProgress.Hide()
		return
	}

	task := d.tasks[d.selectedTask]
	progress := 0.0
	if task.TotalChunks > 0 {
		progress = float64(task.UploadedChunks) / float64(task.TotalChunks)
	}
	d.selectedTaskLabel.SetText(fmt.Sprintf(
		"当前上传任务：%s\nUploadID: %s\n进度：%d / %d | 状态：%s",
		task.FileName,
		task.UploadID,
		task.UploadedChunks,
		task.TotalChunks,
		taskStatusText(task.Status),
	))
	d.selectedTaskProgress.SetValue(progress)
	d.selectedTaskProgress.Show()
}

func (d *DesktopApp) selectedRemoteFile() (transfer.RemoteFile, error) {
	if d.selectedFile < 0 || d.selectedFile >= len(d.files) {
		return transfer.RemoteFile{}, errors.New("请先在文件列表中选中一个文件")
	}
	return d.files[d.selectedFile], nil
}

func (d *DesktopApp) selectedRemoteTask() (transfer.RemoteTask, error) {
	if d.selectedTask < 0 || d.selectedTask >= len(d.tasks) {
		return transfer.RemoteTask{}, errors.New("请先在任务列表中选中一个任务")
	}
	return d.tasks[d.selectedTask], nil
}

func (d *DesktopApp) runAsync(activity string, work func() error, onSuccess func()) {
	d.startBusy(activity)

	go func() {
		err := work()
		fyne.Do(func() {
			d.stopBusy()
			if err != nil {
				d.showError(err)
				return
			}
			if onSuccess != nil {
				onSuccess()
			}
			d.refreshSnapshot()
		})
	}()
}

func (d *DesktopApp) startBusy(activity string) {
	d.activityLabel.SetText(activity)
	d.busyBar.Show()
	d.busyBar.Start()
}

func (d *DesktopApp) stopBusy() {
	d.busyBar.Stop()
	d.busyBar.Hide()
	if strings.TrimSpace(d.activityLabel.Text) == "" {
		d.activityLabel.SetText("后台当前没有正在执行的操作。")
	}
}

func (d *DesktopApp) showError(err error) {
	if err == nil {
		return
	}
	d.setStatus("操作失败：" + err.Error())
	d.activityLabel.SetText("后台当前没有正在执行的操作。")
	d.busyBar.Stop()
	d.busyBar.Hide()
	dialog.ShowError(err, d.window)
	d.refreshSnapshot()
}

func (d *DesktopApp) setStatus(message string) {
	if strings.TrimSpace(message) == "" {
		message = "就绪。"
	}
	d.statusLabel.SetText(message)
}

func (d *DesktopApp) markRefreshed() {
	d.lastRefreshLabel.SetText("最近自动刷新：" + time.Now().Format("2006-01-02 15:04:05"))
}

func formatDeviceItem(item device.RemoteDevice) string {
	return fmt.Sprintf("%s\nID: %s\n类型: %s | 状态: %s | 最近在线: %s", item.DeviceName, item.DeviceID, item.DeviceType, item.Status, emptyAs(item.LastSeenAt, "-"))
}

func formatFileItem(item transfer.RemoteFile) string {
	return fmt.Sprintf("%s\nID: %s\n大小: %d 字节 | 状态: %s", item.FileName, item.FileID, item.FileSize, item.Status)
}

func formatTaskItem(item transfer.RemoteTask) string {
	return fmt.Sprintf("%s\n上传ID: %s\n文件ID: %s | 进度: %d/%d | 状态: %s", item.FileName, item.UploadID, item.FileID, item.UploadedChunks, item.TotalChunks, taskStatusText(item.Status))
}

func applyListHeight(list *widget.List, count int, height float32) {
	if list == nil {
		return
	}
	for i := 0; i < count; i++ {
		list.SetItemHeight(i, height)
	}
}

func emptyAs(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func taskStatusText(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "initialized":
		return "已初始化"
	case "uploading":
		return "上传中"
	case "failed":
		return "失败"
	case "completed":
		return "已完成"
	default:
		return emptyAs(status, "未知")
	}
}
