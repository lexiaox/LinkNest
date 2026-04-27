//go:build android

package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	androidClientVersion = "android-0.1.0"
	mobileDeviceHeight   = 84
	mobileFileHeight     = 72
	mobileTaskHeight     = 92
	systemDownloadDir    = "/storage/emulated/0/Download"
	legacyDownloadDir    = "/sdcard/Download"
)

type MobileApp struct {
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
	targetDeviceID string
	targetSelect   *widget.Select
	targetOptions  map[string]string

	files        []transfer.RemoteFile
	selectedFile int
	fileList     *widget.List
	downloadHint *widget.Label

	tasks                []transfer.RemoteTask
	selectedTask         int
	taskList             *widget.List
	selectedTaskLabel    *widget.Label
	selectedTaskProgress *widget.ProgressBar

	transfers             []transfer.TransferTask
	selectedTransfer      int
	transferList          *widget.List
	selectedTransferLabel *widget.Label

	downloadDirOnce   sync.Once
	cachedDownloadDir string

	autoRefreshStopCh chan struct{}
}

func Launch() error {
	fyneApp := app.NewWithID("top.ledouya.linknest.mobile")
	root := fyneApp.Storage().RootURI().Path()

	svc, err := appsvc.NewWithClientVersion(root, androidClientVersion)
	if err != nil {
		return err
	}

	gui := &MobileApp{
		svc:              svc,
		app:              fyneApp,
		selectedDevice:   -1,
		selectedFile:     -1,
		selectedTask:     -1,
		selectedTransfer: -1,
	}
	gui.window = fyneApp.NewWindow("LinkNest Mobile")
	gui.window.SetContent(gui.buildContent())
	gui.window.SetCloseIntercept(func() {
		gui.stopAutoRefresh()
		gui.svc.StopHeartbeat()
		gui.svc.StopP2P()
		gui.window.Close()
	})
	gui.initDownloadDir()
	gui.refreshSnapshot()
	gui.preloadDataIfReady()
	gui.startAutoRefresh()
	gui.window.ShowAndRun()
	return nil
}

func (m *MobileApp) buildContent() fyne.CanvasObject {
	snapshot := m.svc.Snapshot()

	m.serverEntry = widget.NewEntry()
	m.serverEntry.MultiLine = true
	m.serverEntry.Wrapping = fyne.TextWrapBreak
	m.serverEntry.SetMinRowsVisible(1)
	m.serverEntry.SetText(snapshot.ServerURL)

	m.usernameEntry = widget.NewEntry()
	m.usernameEntry.SetPlaceHolder("用户名")

	m.emailEntry = widget.NewEntry()
	m.emailEntry.SetPlaceHolder("邮箱（注册时必填）")

	m.passwordEntry = widget.NewPasswordEntry()
	m.passwordEntry.SetPlaceHolder("密码")

	m.deviceName = widget.NewEntry()
	m.deviceName.SetPlaceHolder("设备名（默认主机名）")

	m.deviceType = widget.NewEntry()
	m.deviceType.SetText("android")

	m.statusLabel = widget.NewLabel("就绪。")
	m.statusLabel.Wrapping = fyne.TextWrapBreak

	m.activityLabel = widget.NewLabel("后台当前没有正在执行的操作。")
	m.activityLabel.Wrapping = fyne.TextWrapBreak

	m.lastRefreshLabel = widget.NewLabel("最近自动刷新：尚未开始")
	m.lastRefreshLabel.Wrapping = fyne.TextWrapBreak

	m.busyBar = widget.NewProgressBarInfinite()
	m.busyBar.Hide()

	m.snapshotLabel = widget.NewLabel("")
	m.snapshotLabel.Wrapping = fyne.TextWrapBreak

	tabs := container.NewAppTabs(
		container.NewTabItem("账号", m.buildAccountTab()),
		container.NewTabItem("设备", m.buildDeviceTab()),
		container.NewTabItem("文件", m.buildFileTab()),
		container.NewTabItem("上传任务", m.buildTaskTab()),
	)
	tabs.SetTabLocation(container.TabLocationBottom)

	introLabel := widget.NewLabel("移动端直接复用现有 Go 客户端模块，并把本地配置保存在应用沙箱里。")
	introLabel.Wrapping = fyne.TextWrapBreak

	header := container.NewVBox(
		widget.NewLabelWithStyle("LinkNest Android GUI", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		introLabel,
		m.snapshotLabel,
	)

	return container.NewBorder(
		container.NewVBox(header, widget.NewSeparator(), m.busyBar, m.statusLabel, widget.NewSeparator()),
		nil,
		nil,
		nil,
		tabs,
	)
}

func (m *MobileApp) buildAccountTab() fyne.CanvasObject {
	saveServerButton := widget.NewButton("保存服务器地址", func() {
		if err := m.svc.SetServerURL(m.serverEntry.Text); err != nil {
			m.showError(err)
			return
		}
		m.setStatus("服务器地址已保存。")
		m.refreshSnapshot()
	})

	loginButton := widget.NewButton("登录", func() {
		var username string
		m.runAsync("正在登录...", func() error {
			result, err := m.svc.Login(m.usernameEntry.Text, m.passwordEntry.Text)
			if err != nil {
				return err
			}
			username = result.User.Username
			return nil
		}, func() {
			m.setStatus(fmt.Sprintf("登录成功，当前用户：%s", username))
			m.preloadDataIfReady()
		})
	})

	registerButton := widget.NewButton("注册", func() {
		var message string
		m.runAsync("正在注册账号...", func() error {
			result, err := m.svc.Register(m.usernameEntry.Text, m.emailEntry.Text, m.passwordEntry.Text)
			if err != nil {
				return err
			}
			message = fmt.Sprintf("注册成功，当前用户：%s", result.User.Username)
			if strings.TrimSpace(result.Notice) != "" {
				message += "；" + strings.TrimSpace(result.Notice)
			}
			return nil
		}, func() {
			m.setStatus(message)
			m.preloadDataIfReady()
		})
	})

	deleteButton := widget.NewButton("注销账号", func() {
		dialog.ShowConfirm("注销账号", "确认注销当前账号吗？这会删除该账号下的设备、文件和上传任务。", func(ok bool) {
			if !ok {
				return
			}

			var deletedUser string
			m.runAsync("正在注销账号并清理服务器数据...", func() error {
				result, err := m.svc.DeleteAccount(m.passwordEntry.Text)
				if err != nil {
					return err
				}
				deletedUser = result.User.Username
				return nil
			}, func() {
				m.devices = nil
				m.files = nil
				m.tasks = nil
				m.transfers = nil
				m.selectedDevice = -1
				m.selectedFile = -1
				m.selectedTask = -1
				m.selectedTransfer = -1
				m.deviceList.Refresh()
				m.fileList.Refresh()
				m.taskList.Refresh()
				m.transferList.Refresh()
				m.updateSelectedTaskSummary()
				m.updateSelectedTransferSummary()
				m.setStatus(fmt.Sprintf("账号 %s 已注销。", deletedUser))
			})
		}, m.window)
	})

	return container.NewVScroll(mobileStack(
		accountField("服务器", m.serverEntry),
		accountField("用户名", m.usernameEntry),
		accountField("邮箱", m.emailEntry),
		accountField("密码", m.passwordEntry),
		saveServerButton,
		loginButton,
		registerButton,
		deleteButton,
		wrappingLabel("先保存服务器地址，再登录或注册。正式绑定这台手机请到“设备”页点击“绑定当前设备”。"),
	))
}

func (m *MobileApp) buildDeviceTab() fyne.CanvasObject {
	m.deviceList = widget.NewList(
		func() int { return len(m.devices) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapBreak
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatDeviceItem(m.devices[id]))
		},
	)
	m.deviceList.OnSelected = func(id widget.ListItemID) {
		m.selectedDevice = id
	}

	bindButton := widget.NewButton("绑定当前设备", func() {
		var profile device.Profile
		var devices []device.RemoteDevice
		m.runAsync("正在绑定当前设备...", func() error {
			var err error
			profile, err = m.svc.BindCurrentDevice(m.deviceName.Text, m.deviceType.Text)
			if err != nil {
				return err
			}
			devices, err = m.svc.ListDevices()
			return err
		}, func() {
			m.applyDevices(devices)
			m.setStatus(fmt.Sprintf("设备已绑定：%s (%s)", profile.DeviceName, profile.DeviceID))
		})
	})

	refreshButton := widget.NewButton("刷新设备列表", func() {
		var devices []device.RemoteDevice
		m.runAsync("正在刷新设备列表...", func() error {
			var err error
			devices, err = m.svc.ListDevices()
			return err
		}, func() {
			m.applyDevices(devices)
			m.setStatus(fmt.Sprintf("设备列表已刷新，共 %d 台设备。", len(m.devices)))
		})
	})

	startButton := widget.NewButton("开始在线心跳", func() {
		if err := m.svc.StartHeartbeat(); err != nil {
			m.showError(err)
			return
		}
		m.setStatus("在线心跳已启动。")
		m.refreshSnapshot()
	})

	stopButton := widget.NewButton("停止在线心跳", func() {
		m.svc.StopHeartbeat()
		m.setStatus("在线心跳已停止。")
		m.refreshSnapshot()
	})

	startP2PButton := widget.NewButton("启动 P2P 服务", func() {
		var devices []device.RemoteDevice
		m.runAsync("正在启动 P2P 接收服务...", func() error {
			if err := m.svc.StartP2P(); err != nil {
				return err
			}
			var err error
			devices, err = m.svc.ListDevices()
			return err
		}, func() {
			m.applyDevices(devices)
			m.setStatus("P2P 接收服务已启动。")
		})
	})

	stopP2PButton := widget.NewButton("停止 P2P 服务", func() {
		m.svc.StopP2P()
		m.setStatus("P2P 接收服务已停止。")
		m.refreshSnapshot()
	})

	controls := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("设备名", m.deviceName),
			widget.NewFormItem("设备类型", m.deviceType),
		),
		bindButton,
		refreshButton,
		startButton,
		stopButton,
		startP2PButton,
		stopP2PButton,
		widget.NewSeparator(),
	)
	return container.NewBorder(controls, nil, nil, nil, m.deviceList)
}

func (m *MobileApp) buildFileTab() fyne.CanvasObject {
	m.targetSelect = widget.NewSelect(nil, func(value string) {
		m.targetDeviceID = m.targetDeviceIDFromOption(value)
	})
	m.targetSelect.PlaceHolder = "选择目标在线设备"

	m.fileList = widget.NewList(
		func() int { return len(m.files) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapBreak
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatFileItem(m.files[id]))
		},
	)
	m.fileList.OnSelected = func(id widget.ListItemID) {
		m.selectedFile = id
	}

	refreshButton := widget.NewButton("刷新文件列表", func() {
		var files []transfer.RemoteFile
		m.runAsync("正在刷新文件列表...", func() error {
			var err error
			files, err = m.svc.ListFiles()
			return err
		}, func() {
			m.applyFiles(files)
			m.setStatus(fmt.Sprintf("文件列表已刷新，共 %d 个文件。", len(m.files)))
		})
	})

	uploadButton := widget.NewButton("上传文件", func() {
		dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				m.showError(err)
				return
			}
			if reader == nil {
				return
			}

			tempPath, copyErr := m.persistUploadSelection(reader)
			if copyErr != nil {
				m.showError(copyErr)
				return
			}

			var files []transfer.RemoteFile
			var tasks []transfer.RemoteTask
			m.runAsync("正在上传文件...", func() error {
				if err := m.svc.Upload(tempPath); err != nil {
					return err
				}
				var err error
				files, err = m.svc.ListFiles()
				if err != nil {
					return err
				}
				tasks, err = m.svc.ListTasks()
				return err
			}, func() {
				m.applyFiles(files)
				m.applyTasks(tasks)
				m.setStatus(fmt.Sprintf("上传完成：%s", filepath.Base(tempPath)))
			})
		}, m.window).Show()
	})

	downloadButton := widget.NewButton("下载选中文件", func() {
		file, err := m.selectedRemoteFile()
		if err != nil {
			m.showError(err)
			return
		}

		path, pathErr := m.defaultDownloadPath(file.FileName)
		if pathErr != nil {
			m.showError(pathErr)
			return
		}

		m.runAsync("正在下载文件...", func() error {
			return m.svc.Download(file.FileID, path)
		}, func() {
			m.setStatus(fmt.Sprintf("下载完成：%s -> %s", file.FileName, path))
			m.setDownloadHint(path)
		})
	})

	deleteButton := widget.NewButton("删除选中文件", func() {
		file, err := m.selectedRemoteFile()
		if err != nil {
			m.showError(err)
			return
		}

		dialog.ShowConfirm("删除文件", fmt.Sprintf("确认删除文件 %s 吗？", file.FileName), func(ok bool) {
			if !ok {
				return
			}

			var files []transfer.RemoteFile
			var tasks []transfer.RemoteTask
			m.runAsync("正在删除文件...", func() error {
				if err := m.svc.DeleteFile(file.FileID); err != nil {
					return err
				}
				var err error
				files, err = m.svc.ListFiles()
				if err != nil {
					return err
				}
				tasks, err = m.svc.ListTasks()
				return err
			}, func() {
				m.applyFiles(files)
				m.applyTasks(tasks)
				m.setStatus(fmt.Sprintf("文件已删除：%s", file.FileName))
			})
		}, m.window)
	})

	sendButton := widget.NewButton("V2 发送到目标设备", func() {
		target, err := m.selectedTargetDevice()
		if err != nil {
			m.showError(err)
			return
		}
		dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				m.showError(err)
				return
			}
			if reader == nil {
				return
			}

			tempPath, copyErr := m.persistUploadSelection(reader)
			if copyErr != nil {
				m.showError(copyErr)
				return
			}

			var tasks []transfer.RemoteTask
			var transfers []transfer.TransferTask
			m.runAsync("正在发起 V2 传输...", func() error {
				if err := m.svc.SendTransfer(tempPath, target.DeviceID); err != nil {
					return err
				}
				var err error
				tasks, err = m.svc.ListTasks()
				if err != nil {
					return err
				}
				transfers, err = m.svc.ListTransfers()
				return err
			}, func() {
				m.applyTasksAndTransfers(tasks, transfers)
				m.setStatus(fmt.Sprintf("传输已完成：%s -> %s", filepath.Base(tempPath), target.DeviceName))
			})
		}, m.window).Show()
	})

	m.downloadHint = wrappingLabel("下载保存位置：系统 Downloads（可能为 /storage/emulated/0/Download 或 /sdcard/Download）。如果系统拒绝写入，会自动回退到应用沙箱 Documents。")

	controls := container.NewVBox(
		refreshButton,
		uploadButton,
		accountField("P2P 目标设备", m.targetSelect),
		sendButton,
		downloadButton,
		deleteButton,
		m.downloadHint,
		widget.NewSeparator(),
	)
	return container.NewBorder(controls, nil, nil, nil, m.fileList)
}

func (m *MobileApp) buildTaskTab() fyne.CanvasObject {
	m.transferList = widget.NewList(
		func() int { return len(m.transfers) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapBreak
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatTransferItem(m.transfers[id]))
		},
	)
	m.transferList.OnSelected = func(id widget.ListItemID) {
		m.selectedTransfer = id
		m.updateSelectedTransferSummary()
	}

	m.taskList = widget.NewList(
		func() int { return len(m.tasks) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapBreak
			return label
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(formatTaskItem(m.tasks[id]))
		},
	)
	m.taskList.OnSelected = func(id widget.ListItemID) {
		m.selectedTask = id
		m.updateSelectedTaskSummary()
	}

	m.selectedTaskLabel = widget.NewLabel("请选择一条上传任务查看详情。这里显示的是每次文件上传或续传的记录。")
	m.selectedTaskLabel.Wrapping = fyne.TextWrapBreak

	m.selectedTaskProgress = widget.NewProgressBar()
	m.selectedTaskProgress.Min = 0
	m.selectedTaskProgress.Max = 1
	m.selectedTaskProgress.Hide()

	m.selectedTransferLabel = widget.NewLabel("请选择一条 V2 传输任务查看 P2P/cloud 路径和失败原因。")
	m.selectedTransferLabel.Wrapping = fyne.TextWrapBreak

	refreshButton := widget.NewButton("刷新任务列表", func() {
		var tasks []transfer.RemoteTask
		var transfers []transfer.TransferTask
		m.runAsync("正在刷新任务列表...", func() error {
			var err error
			tasks, err = m.svc.ListTasks()
			if err != nil {
				return err
			}
			transfers, err = m.svc.ListTransfers()
			return err
		}, func() {
			m.applyTasksAndTransfers(tasks, transfers)
			m.setStatus(fmt.Sprintf("任务列表已刷新，V1 %d 条，V2 %d 条。", len(m.tasks), len(m.transfers)))
		})
	})

	resumeButton := widget.NewButton("继续选中任务", func() {
		task, err := m.selectedRemoteTask()
		if err != nil {
			m.showError(err)
			return
		}

		var tasks []transfer.RemoteTask
		var files []transfer.RemoteFile
		m.runAsync("正在继续上传任务...", func() error {
			if err := m.svc.ResumeTask(task.UploadID); err != nil {
				return err
			}
			var err error
			tasks, err = m.svc.ListTasks()
			if err != nil {
				return err
			}
			files, err = m.svc.ListFiles()
			return err
		}, func() {
			m.applyTasks(tasks)
			m.applyFiles(files)
			m.setStatus(fmt.Sprintf("上传任务已继续：%s", task.UploadID))
		})
	})

	controls := container.NewVBox(
		m.selectedTransferLabel,
		m.selectedTaskLabel,
		m.selectedTaskProgress,
		refreshButton,
		resumeButton,
		widget.NewSeparator(),
	)
	return container.NewBorder(
		controls,
		nil,
		nil,
		nil,
		container.NewVSplit(
			container.NewBorder(wrappingLabel("V2 传输任务"), nil, nil, nil, m.transferList),
			container.NewBorder(wrappingLabel("V1 上传任务"), nil, nil, nil, m.taskList),
		),
	)
}

func (m *MobileApp) initDownloadDir() {
	m.downloadDirOnce.Do(func() {
		if dir, ok := writableDownloadDir(); ok {
			m.cachedDownloadDir = dir
		}
	})
}

func (m *MobileApp) defaultDownloadPath(fileName string) (string, error) {
	name := safeFileName(fileName)
	if m.cachedDownloadDir != "" {
		return filepath.Join(m.cachedDownloadDir, name), nil
	}

	dir := filepath.Join(m.svc.Root(), "Documents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func (m *MobileApp) persistUploadSelection(reader fyne.URIReadCloser) (string, error) {
	defer reader.Close()

	importDir := filepath.Join(m.svc.Root(), "imports")
	if err := os.MkdirAll(importDir, 0755); err != nil {
		return "", err
	}

	name := reader.URI().Name()
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("upload-%d.bin", time.Now().UnixNano())
	}
	targetPath := filepath.Join(importDir, name)

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, reader); err != nil {
		return "", err
	}
	return targetPath, nil
}

func (m *MobileApp) preloadDataIfReady() {
	snapshot := m.svc.Snapshot()
	if !snapshot.HasToken {
		return
	}
	_ = m.refreshDevices(true)
	_ = m.refreshFiles(true)
	_ = m.refreshTasks(true)
}

func (m *MobileApp) startAutoRefresh() {
	if m.autoRefreshStopCh != nil {
		return
	}

	stopCh := make(chan struct{})
	m.autoRefreshStopCh = stopCh

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

			snapshot := m.svc.Snapshot()
			if !snapshot.HasToken {
				continue
			}

			cycle++

			devices, devErr := m.svc.ListDevices()
			tasks, taskErr := m.svc.ListTasks()
			transfers, transferErr := m.svc.ListTransfers()
			var files []transfer.RemoteFile
			fetchedFiles := false
			var fileErr error
			if cycle%2 == 0 {
				files, fileErr = m.svc.ListFiles()
				fetchedFiles = true
			}

			now := time.Now()
			fyne.Do(func() {
				if devErr == nil {
					m.devices = devices
					m.deviceList.Refresh()
					applyListHeight(m.deviceList, len(m.devices), mobileDeviceHeight)
					m.refreshTargetDeviceOptions()
				}
				if taskErr == nil {
					m.tasks = tasks
					m.taskList.Refresh()
					applyListHeight(m.taskList, len(m.tasks), mobileTaskHeight)
					m.updateSelectedTaskSummary()
				}
				if transferErr == nil {
					m.transfers = transfers
					m.transferList.Refresh()
					applyListHeight(m.transferList, len(m.transfers), mobileTaskHeight)
					m.updateSelectedTransferSummary()
				}
				if fileErr == nil && fetchedFiles {
					m.applyFiles(files)
				}
				m.lastRefreshLabel.SetText("最近自动刷新：" + now.Format("2006-01-02 15:04:05"))
				m.refreshSnapshot()
			})
		}
	}()
}

func (m *MobileApp) stopAutoRefresh() {
	if m.autoRefreshStopCh == nil {
		return
	}
	close(m.autoRefreshStopCh)
	m.autoRefreshStopCh = nil
}

func (m *MobileApp) refreshDevices(silent bool) error {
	items, err := m.svc.ListDevices()
	if err != nil {
		if !silent {
			m.showError(err)
		}
		return err
	}
	m.devices = items
	m.selectedDevice = -1
	m.deviceList.Refresh()
	applyListHeight(m.deviceList, len(m.devices), mobileDeviceHeight)
	m.refreshTargetDeviceOptions()
	m.markRefreshed()
	return nil
}

func (m *MobileApp) refreshFiles(silent bool) error {
	items, err := m.svc.ListFiles()
	if err != nil {
		if !silent {
			m.showError(err)
		}
		return err
	}
	m.files = items
	m.selectedFile = -1
	m.fileList.Refresh()
	applyListHeight(m.fileList, len(m.files), mobileFileHeight)
	m.markRefreshed()
	return nil
}

func (m *MobileApp) refreshTasks(silent bool) error {
	items, err := m.svc.ListTasks()
	if err != nil {
		if !silent {
			m.showError(err)
		}
		return err
	}
	transfers, err := m.svc.ListTransfers()
	if err != nil {
		if !silent {
			m.showError(err)
		}
		return err
	}
	m.tasks = items
	m.transfers = transfers
	m.selectedTask = -1
	m.selectedTransfer = -1
	m.taskList.Refresh()
	m.transferList.Refresh()
	applyListHeight(m.taskList, len(m.tasks), mobileTaskHeight)
	applyListHeight(m.transferList, len(m.transfers), mobileTaskHeight)
	m.updateSelectedTaskSummary()
	m.updateSelectedTransferSummary()
	m.markRefreshed()
	return nil
}

func (m *MobileApp) applyDevices(items []device.RemoteDevice) {
	m.devices = items
	m.selectedDevice = -1
	m.deviceList.Refresh()
	applyListHeight(m.deviceList, len(m.devices), mobileDeviceHeight)
	m.refreshTargetDeviceOptions()
	m.markRefreshed()
}

func (m *MobileApp) applyFiles(items []transfer.RemoteFile) {
	m.files = items
	m.selectedFile = -1
	m.fileList.Refresh()
	applyListHeight(m.fileList, len(m.files), mobileFileHeight)
	m.markRefreshed()
}

func (m *MobileApp) applyTasks(items []transfer.RemoteTask) {
	m.tasks = items
	m.selectedTask = -1
	m.taskList.Refresh()
	applyListHeight(m.taskList, len(m.tasks), mobileTaskHeight)
	m.updateSelectedTaskSummary()
	m.markRefreshed()
}

func (m *MobileApp) applyTasksAndTransfers(tasks []transfer.RemoteTask, transfers []transfer.TransferTask) {
	m.tasks = tasks
	m.transfers = transfers
	m.selectedTask = -1
	m.selectedTransfer = -1
	m.taskList.Refresh()
	m.transferList.Refresh()
	applyListHeight(m.taskList, len(m.tasks), mobileTaskHeight)
	applyListHeight(m.transferList, len(m.transfers), mobileTaskHeight)
	m.updateSelectedTaskSummary()
	m.updateSelectedTransferSummary()
	m.markRefreshed()
}

func (m *MobileApp) refreshSnapshot() {
	snapshot := m.svc.Snapshot()

	tokenText := "未登录"
	if snapshot.HasToken {
		tokenText = "已登录"
	}

	deviceText := "未绑定"
	if strings.TrimSpace(snapshot.DeviceID) != "" {
		deviceText = fmt.Sprintf("%s (%s)", snapshot.DeviceName, shortID(snapshot.DeviceID))
	}

	heartbeatText := "未运行"
	if snapshot.HeartbeatRunning {
		heartbeatText = "运行中"
	}
	if strings.TrimSpace(snapshot.HeartbeatError) != "" {
		heartbeatText += "；最近错误：" + strings.TrimSpace(snapshot.HeartbeatError)
	}

	m.snapshotLabel.SetText(fmt.Sprintf(
		"服务器：%s\n登录状态：%s\n当前设备：%s\n在线心跳：%s\nP2P：enabled=%t running=%t port=%d fallback=%t\n本地目录：%s",
		emptyAs(snapshot.ServerURL, "未设置"),
		tokenText,
		deviceText,
		heartbeatText,
		snapshot.P2PEnabled,
		snapshot.P2PRunning,
		snapshot.P2PPort,
		snapshot.FallbackToCloud,
		compactPath(m.svc.Root()),
	))
}

func (m *MobileApp) updateSelectedTaskSummary() {
	if m.selectedTask < 0 || m.selectedTask >= len(m.tasks) {
		m.selectedTaskLabel.SetText("请选择一条上传任务查看详情。自动刷新会保持上传进度为最新状态。")
		m.selectedTaskProgress.SetValue(0)
		m.selectedTaskProgress.Hide()
		return
	}

	task := m.tasks[m.selectedTask]
	progress := 0.0
	if task.TotalChunks > 0 {
		progress = float64(task.UploadedChunks) / float64(task.TotalChunks)
	}
	m.selectedTaskLabel.SetText(fmt.Sprintf(
		"当前上传任务：%s\n上传ID: %s\n进度：%d / %d | 状态：%s",
		task.FileName,
		shortID(task.UploadID),
		task.UploadedChunks,
		task.TotalChunks,
		taskStatusText(task.Status),
	))
	m.selectedTaskProgress.SetValue(progress)
	m.selectedTaskProgress.Show()
}

func (m *MobileApp) updateSelectedTransferSummary() {
	if m.selectedTransfer < 0 || m.selectedTransfer >= len(m.transfers) {
		m.selectedTransferLabel.SetText("请选择一条 V2 传输任务查看 P2P/cloud 路径和失败原因。")
		return
	}

	task := m.transfers[m.selectedTransfer]
	m.selectedTransferLabel.SetText(fmt.Sprintf(
		"当前传输：%s\n传输ID: %s\n设备：%s -> %s\n路径：%s/%s\n状态：%s\n失败原因：%s %s",
		task.FileName,
		shortID(task.TransferID),
		shortID(task.SourceDeviceID),
		shortID(task.TargetDeviceID),
		emptyAs(task.PreferredRoute, "-"),
		emptyAs(task.ActualRoute, "-"),
		taskStatusText(task.Status),
		emptyAs(task.ErrorCode, "-"),
		emptyAs(task.ErrorMessage, ""),
	))
}

func (m *MobileApp) selectedRemoteDevice() (device.RemoteDevice, error) {
	if m.selectedDevice < 0 || m.selectedDevice >= len(m.devices) {
		return device.RemoteDevice{}, errors.New("请先在设备列表中选中目标在线设备")
	}
	return m.devices[m.selectedDevice], nil
}

func (m *MobileApp) selectedTargetDevice() (device.RemoteDevice, error) {
	targetID := strings.TrimSpace(m.targetDeviceID)
	if targetID == "" {
		return device.RemoteDevice{}, errors.New("请先在文件页选择 P2P 目标在线设备")
	}
	for _, item := range m.devices {
		if item.DeviceID == targetID {
			return item, nil
		}
	}
	return device.RemoteDevice{}, errors.New("目标设备不在线，请刷新设备列表后重新选择")
}

func (m *MobileApp) selectedRemoteFile() (transfer.RemoteFile, error) {
	if m.selectedFile < 0 || m.selectedFile >= len(m.files) {
		return transfer.RemoteFile{}, errors.New("请先在文件列表中选中一个文件")
	}
	return m.files[m.selectedFile], nil
}

func (m *MobileApp) selectedRemoteTask() (transfer.RemoteTask, error) {
	if m.selectedTask < 0 || m.selectedTask >= len(m.tasks) {
		return transfer.RemoteTask{}, errors.New("请先在任务列表中选中一个任务")
	}
	return m.tasks[m.selectedTask], nil
}

func (m *MobileApp) runAsync(activity string, work func() error, onSuccess func()) {
	m.startBusy(activity)

	go func() {
		err := work()
		fyne.Do(func() {
			m.stopBusy()
			if err != nil {
				m.showError(err)
				return
			}
			if onSuccess != nil {
				onSuccess()
			}
			m.refreshSnapshot()
		})
	}()
}

func (m *MobileApp) startBusy(activity string) {
	m.activityLabel.SetText(activity)
	m.statusLabel.SetText(activity)
	m.busyBar.Show()
	m.busyBar.Start()
}

func (m *MobileApp) stopBusy() {
	m.busyBar.Stop()
	m.busyBar.Hide()
	if strings.TrimSpace(m.activityLabel.Text) == "" {
		m.activityLabel.SetText("后台当前没有正在执行的操作。")
	}
}

func (m *MobileApp) showError(err error) {
	if err == nil {
		return
	}
	m.setStatus("操作失败：" + err.Error())
	m.activityLabel.SetText("后台当前没有正在执行的操作。")
	m.busyBar.Stop()
	m.busyBar.Hide()
	dialog.ShowError(err, m.window)
	m.refreshSnapshot()
}

func (m *MobileApp) setStatus(message string) {
	if strings.TrimSpace(message) == "" {
		message = "就绪。"
	}
	m.statusLabel.SetText(message)
}

func (m *MobileApp) markRefreshed() {
	m.lastRefreshLabel.SetText("最近自动刷新：" + time.Now().Format("2006-01-02 15:04:05"))
}

func formatDeviceItem(item device.RemoteDevice) string {
	return fmt.Sprintf("%s\nID: %s\n类型: %s | 状态: %s | P2P: %t %s:%d | 最近在线: %s", item.DeviceName, shortID(item.DeviceID), item.DeviceType, item.Status, item.P2PEnabled, emptyAs(item.P2PProtocol, "http"), item.P2PPort, emptyAs(item.LastSeenAt, "-"))
}

func formatFileItem(item transfer.RemoteFile) string {
	return fmt.Sprintf("%s\nID: %s\n大小: %d 字节 | 状态: %s", item.FileName, shortID(item.FileID), item.FileSize, item.Status)
}

func formatTaskItem(item transfer.RemoteTask) string {
	return fmt.Sprintf("%s\n上传ID: %s\n文件ID: %s | 进度: %d/%d | 状态: %s", item.FileName, shortID(item.UploadID), shortID(item.FileID), item.UploadedChunks, item.TotalChunks, taskStatusText(item.Status))
}

func formatTransferItem(item transfer.TransferTask) string {
	return fmt.Sprintf("%s\n传输ID: %s\n设备: %s -> %s | 路径: %s/%s | 状态: %s | 错误: %s", item.FileName, shortID(item.TransferID), shortID(item.SourceDeviceID), shortID(item.TargetDeviceID), emptyAs(item.PreferredRoute, "-"), emptyAs(item.ActualRoute, "-"), taskStatusText(item.Status), emptyAs(item.ErrorCode, "-"))
}

func (m *MobileApp) refreshTargetDeviceOptions() {
	if m.targetSelect == nil {
		return
	}
	currentID := strings.TrimSpace(m.svc.Snapshot().DeviceID)
	options := make([]string, 0, len(m.devices))
	optionMap := make(map[string]string, len(m.devices))
	selected := ""
	for _, item := range m.devices {
		if item.DeviceID == "" || item.DeviceID == currentID {
			continue
		}
		option := deviceOption(item.DeviceName, item.DeviceID, item.P2PEnabled, item.P2PPort)
		options = append(options, option)
		optionMap[option] = item.DeviceID
		if item.DeviceID == m.targetDeviceID {
			selected = option
		}
	}
	m.targetOptions = optionMap
	m.targetSelect.Options = options
	if selected != "" {
		m.targetSelect.SetSelected(selected)
	} else {
		m.targetDeviceID = ""
		m.targetSelect.ClearSelected()
	}
	m.targetSelect.Refresh()
}

func deviceOption(name string, deviceID string, p2pEnabled bool, p2pPort int) string {
	p2pState := "cloud fallback"
	if p2pEnabled && p2pPort > 0 {
		p2pState = fmt.Sprintf("p2p:%d", p2pPort)
	}
	return fmt.Sprintf("%s | %s | %s", emptyAs(name, "未命名设备"), deviceID, p2pState)
}

func (m *MobileApp) targetDeviceIDFromOption(option string) string {
	if m.targetOptions != nil {
		if deviceID := strings.TrimSpace(m.targetOptions[option]); deviceID != "" {
			return deviceID
		}
	}
	return strings.TrimSpace(option)
}

func applyListHeight(list *widget.List, count int, height float32) {
	if list == nil {
		return
	}
	if count <= 0 {
		list.SetItemHeight(0, height)
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

func safeFileName(value string) string {
	name := filepath.Base(strings.TrimSpace(value))
	if name == "." || name == ".." || name == string(filepath.Separator) || name == "" {
		return fmt.Sprintf("linknest-download-%d", time.Now().UnixNano())
	}
	return name
}

func writableDownloadDir() (string, bool) {
	for _, dir := range []string{systemDownloadDir, legacyDownloadDir} {
		if canWriteDir(dir) {
			return dir, true
		}
	}
	return "", false
}

func canWriteDir(dir string) (ok bool) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}

	probe, err := os.CreateTemp(dir, ".linknest-write-test-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	defer func() {
		closeErr := probe.Close()
		removeErr := os.Remove(name)
		if closeErr != nil || removeErr != nil {
			ok = false
		}
	}()
	return true
}

func (m *MobileApp) setDownloadHint(path string) {
	if m.downloadHint == nil {
		return
	}
	if isPathInDir(path, systemDownloadDir) || isPathInDir(path, legacyDownloadDir) {
		m.downloadHint.SetText("最近下载位置：系统 Downloads\n" + path)
		return
	}
	m.downloadHint.SetText("最近下载位置：应用沙箱 Documents（系统拒绝写入公共 Downloads）\n" + path)
}

func isPathInDir(path string, dir string) bool {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	cleanDir := filepath.Clean(strings.TrimSpace(dir))
	if cleanPath == cleanDir {
		return true
	}

	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func accountField(labelText string, input fyne.CanvasObject) fyne.CanvasObject {
	return mobileStack(wrappingLabel(labelText), input)
}

func mobileStack(objects ...fyne.CanvasObject) *fyne.Container {
	return container.New(&mobileStackLayout{gap: 8}, objects...)
}

type mobileStackLayout struct {
	gap float32
}

func (l *mobileStackLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, object := range objects {
		if !object.Visible() {
			continue
		}
		height := object.MinSize().Height
		object.Move(fyne.NewPos(0, y))
		object.Resize(fyne.NewSize(size.Width, height))
		y += height + l.gap
	}
}

func (l *mobileStackLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	height := float32(0)
	visible := 0
	for _, object := range objects {
		if !object.Visible() {
			continue
		}
		height += object.MinSize().Height
		visible++
	}
	if visible > 1 {
		height += float32(visible-1) * l.gap
	}
	return fyne.NewSize(1, height)
}

func wrappingLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapBreak
	return label
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 18 {
		return emptyAs(value, "-")
	}
	return value[:8] + "..." + value[len(value)-6:]
}

func compactPath(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 36 {
		return emptyAs(value, "-")
	}

	base := filepath.Base(value)
	parent := filepath.Base(filepath.Dir(value))
	if base == "." || base == string(filepath.Separator) {
		return value
	}
	if parent == "." || parent == string(filepath.Separator) {
		return "..." + string(filepath.Separator) + base
	}
	return "..." + string(filepath.Separator) + filepath.Join(parent, base)
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
