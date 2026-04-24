//go:build windows

package ui

import (
	"errors"
	"fmt"
	"strings"

	"linknest/client/desktop/internal/appsvc"
	"linknest/client/internal/device"
	"linknest/client/internal/transfer"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
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
	statusLabel   *widget.Label
	snapshotLabel *widget.Label

	devices        []device.RemoteDevice
	selectedDevice int
	deviceList     *widget.List

	files        []transfer.RemoteFile
	selectedFile int
	fileList     *widget.List

	tasks        []transfer.RemoteTask
	selectedTask int
	taskList     *widget.List
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
	gui.window.Resize(fyne.NewSize(1180, 760))
	gui.window.SetContent(gui.buildContent())
	gui.refreshSnapshot()
	gui.preloadDataIfReady()
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

	d.snapshotLabel = widget.NewLabel("")
	d.snapshotLabel.Wrapping = fyne.TextWrapWord

	accountTab := container.NewTabItem("账号", d.buildAccountTab())
	deviceTab := container.NewTabItem("设备", d.buildDeviceTab())
	fileTab := container.NewTabItem("文件", d.buildFileTab())
	taskTab := container.NewTabItem("任务", d.buildTaskTab())

	tabs := container.NewAppTabs(accountTab, deviceTab, fileTab, taskTab)
	tabs.SetTabLocation(container.TabLocationTop)

	header := container.NewVBox(
		widget.NewLabelWithStyle("LinkNest Windows GUI", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("桌面端直接复用现有 Go 客户端模块，不再通过 CLI 子进程调用。"),
		d.snapshotLabel,
	)

	return container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), d.statusLabel),
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
		result, err := d.svc.Login(d.usernameEntry.Text, d.passwordEntry.Text)
		if err != nil {
			d.showError(err)
			return
		}
		d.setStatus(fmt.Sprintf("登录成功，当前用户：%s", result.User.Username))
		d.refreshSnapshot()
		d.preloadDataIfReady()
	})

	registerButton := widget.NewButton("注册", func() {
		result, err := d.svc.Register(d.usernameEntry.Text, d.emailEntry.Text, d.passwordEntry.Text)
		if err != nil {
			d.showError(err)
			return
		}
		message := fmt.Sprintf("注册成功，当前用户：%s", result.User.Username)
		if strings.TrimSpace(result.Notice) != "" {
			message = message + "；" + strings.TrimSpace(result.Notice)
		}
		d.setStatus(message)
		d.refreshSnapshot()
		d.preloadDataIfReady()
	})

	deleteButton := widget.NewButton("注销账号", func() {
		dialog.ShowConfirm("注销账号", "确认注销当前账号吗？这会删除该账号下的设备、文件和上传任务。", func(ok bool) {
			if !ok {
				return
			}
			result, err := d.svc.DeleteAccount(d.passwordEntry.Text)
			if err != nil {
				d.showError(err)
				return
			}
			d.devices = nil
			d.files = nil
			d.tasks = nil
			d.selectedDevice = -1
			d.selectedFile = -1
			d.selectedTask = -1
			d.deviceList.Refresh()
			d.fileList.Refresh()
			d.taskList.Refresh()
			d.setStatus(fmt.Sprintf("账号 %s 已注销，服务器数据已清理。", result.User.Username))
			d.refreshSnapshot()
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
		profile, err := d.svc.BindCurrentDevice(d.deviceName.Text, d.deviceType.Text)
		if err != nil {
			d.showError(err)
			return
		}
		d.setStatus(fmt.Sprintf("设备已绑定：%s (%s)", profile.DeviceName, profile.DeviceID))
		d.refreshSnapshot()
		d.refreshDevices()
	})

	refreshButton := widget.NewButton("刷新设备列表", func() {
		d.refreshDevices()
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
		d.refreshFiles()
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
			if err := d.svc.Upload(path); err != nil {
				d.showError(err)
				return
			}
			d.setStatus(fmt.Sprintf("上传完成：%s", path))
			d.refreshFiles()
			d.refreshTasks()
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
			if err := d.svc.Download(file.FileID, path); err != nil {
				d.showError(err)
				return
			}
			d.setStatus(fmt.Sprintf("下载完成：%s -> %s", file.FileName, path))
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
			if err := d.svc.DeleteFile(file.FileID); err != nil {
				d.showError(err)
				return
			}
			d.setStatus(fmt.Sprintf("文件已删除：%s", file.FileName))
			d.refreshFiles()
			d.refreshTasks()
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
	}

	refreshButton := widget.NewButton("刷新任务列表", func() {
		d.refreshTasks()
	})

	resumeButton := widget.NewButton("继续选中任务", func() {
		task, err := d.selectedRemoteTask()
		if err != nil {
			d.showError(err)
			return
		}
		if err := d.svc.ResumeTask(task.UploadID); err != nil {
			d.showError(err)
			return
		}
		d.setStatus(fmt.Sprintf("任务已继续：%s", task.UploadID))
		d.refreshTasks()
		d.refreshFiles()
	})

	return container.NewBorder(
		container.NewVBox(
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
	d.refreshDevices()
	d.refreshFiles()
	d.refreshTasks()
}

func (d *DesktopApp) refreshDevices() {
	items, err := d.svc.ListDevices()
	if err != nil {
		d.showError(err)
		return
	}
	d.devices = items
	d.selectedDevice = -1
	d.deviceList.Refresh()
	d.setStatus(fmt.Sprintf("设备列表已刷新，共 %d 台设备。", len(items)))
}

func (d *DesktopApp) refreshFiles() {
	items, err := d.svc.ListFiles()
	if err != nil {
		d.showError(err)
		return
	}
	d.files = items
	d.selectedFile = -1
	d.fileList.Refresh()
	d.setStatus(fmt.Sprintf("文件列表已刷新，共 %d 个文件。", len(items)))
}

func (d *DesktopApp) refreshTasks() {
	items, err := d.svc.ListTasks()
	if err != nil {
		d.showError(err)
		return
	}
	d.tasks = items
	d.selectedTask = -1
	d.taskList.Refresh()
	d.setStatus(fmt.Sprintf("任务列表已刷新，共 %d 条任务。", len(items)))
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
		heartbeatText = heartbeatText + "；最近错误：" + strings.TrimSpace(snapshot.HeartbeatError)
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

func (d *DesktopApp) showError(err error) {
	if err == nil {
		return
	}
	d.setStatus("操作失败：" + err.Error())
	dialog.ShowError(err, d.window)
	d.refreshSnapshot()
}

func (d *DesktopApp) setStatus(message string) {
	if strings.TrimSpace(message) == "" {
		message = "就绪。"
	}
	d.statusLabel.SetText(message)
}

func formatDeviceItem(item device.RemoteDevice) string {
	return fmt.Sprintf("%s\nID: %s\n类型: %s | 状态: %s | 最近在线: %s", item.DeviceName, item.DeviceID, item.DeviceType, item.Status, emptyAs(item.LastSeenAt, "-"))
}

func formatFileItem(item transfer.RemoteFile) string {
	return fmt.Sprintf("%s\nID: %s\n大小: %d 字节 | 状态: %s", item.FileName, item.FileID, item.FileSize, item.Status)
}

func formatTaskItem(item transfer.RemoteTask) string {
	return fmt.Sprintf("%s\nUploadID: %s\n文件ID: %s | 进度: %d/%d | 状态: %s", item.FileName, item.UploadID, item.FileID, item.UploadedChunks, item.TotalChunks, item.Status)
}

func emptyAs(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
