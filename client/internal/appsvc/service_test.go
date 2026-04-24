package appsvc

import (
	"testing"
	"time"

	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/device"
)

func TestSetServerURLPersistsConfig(t *testing.T) {
	root := t.TempDir()

	service, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.SetServerURL(" http://127.0.0.1:8080/ "); err != nil {
		t.Fatalf("SetServerURL() error = %v", err)
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.ServerURL, "http://127.0.0.1:8080"; got != want {
		t.Fatalf("server URL = %q, want %q", got, want)
	}
}

func TestStartHeartbeatRequiresLogin(t *testing.T) {
	root := t.TempDir()

	service, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.StartHeartbeat(); err == nil {
		t.Fatal("StartHeartbeat() error = nil, want token validation error")
	}
}

func TestStartHeartbeatRequiresBoundDevice(t *testing.T) {
	root := t.TempDir()

	cfg := clientconfig.Default()
	cfg.ServerURL = "http://example.com"
	cfg.Token = "token"
	if err := clientconfig.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if err := clientconfig.Save(root, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := device.Save(root, device.Profile{
		DeviceID:      cfg.Device.DeviceID,
		DeviceName:    cfg.Device.DeviceName,
		DeviceType:    cfg.Device.DeviceType,
		ClientVersion: cfg.Device.ClientVersion,
	}); err != nil {
		t.Fatalf("device.Save() error = %v", err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := service.StartHeartbeat(); err == nil {
		t.Fatal("StartHeartbeat() error = nil, want device profile error")
	}
}

func TestBindCurrentDevicePersistsDeviceConfig(t *testing.T) {
	root := t.TempDir()

	cfg := clientconfig.Default()
	cfg.ServerURL = "http://example.com"
	cfg.Token = "token"
	if err := clientconfig.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if err := clientconfig.Save(root, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := device.Save(root, device.Profile{
		DeviceID:      cfg.Device.DeviceID,
		DeviceName:    cfg.Device.DeviceName,
		DeviceType:    cfg.Device.DeviceType,
		ClientVersion: cfg.Device.ClientVersion,
	}); err != nil {
		t.Fatalf("device.Save() error = %v", err)
	}

	service, err := NewWithClientVersion(root, "desktop-0.1.0")
	if err != nil {
		t.Fatalf("NewWithClientVersion() error = %v", err)
	}

	originalRegister := registerDevice
	registerDevice = func(baseURL string, token string, profile device.Profile) error {
		return nil
	}
	defer func() {
		registerDevice = originalRegister
	}()

	profile, err := service.BindCurrentDevice("Desktop Test", "windows")
	if err != nil {
		t.Fatalf("BindCurrentDevice() error = %v", err)
	}
	if profile.DeviceID == "" {
		t.Fatal("BindCurrentDevice() returned empty device ID")
	}

	snapshot := service.Snapshot()
	if snapshot.DeviceID == "" || snapshot.DeviceName != "Desktop Test" || snapshot.DeviceType != "windows" {
		t.Fatalf("unexpected snapshot after bind: %+v", snapshot)
	}
}

func TestStopHeartbeatCancelsRunningWorker(t *testing.T) {
	root := t.TempDir()

	cfg := clientconfig.Default()
	cfg.ServerURL = "http://example.com"
	cfg.Token = "token"
	cfg.Device = clientconfig.DeviceConfig{
		DeviceID:      "device-1",
		DeviceName:    "demo-device",
		DeviceType:    "windows",
		ClientVersion: "desktop-0.1.0",
	}
	if err := clientconfig.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if err := clientconfig.Save(root, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := device.Save(root, device.Profile{
		DeviceID:      cfg.Device.DeviceID,
		DeviceName:    cfg.Device.DeviceName,
		DeviceType:    cfg.Device.DeviceType,
		ClientVersion: cfg.Device.ClientVersion,
	}); err != nil {
		t.Fatalf("device.Save() error = %v", err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	started := make(chan struct{})
	stopped := make(chan struct{})
	service.heartbeatFn = func(serverURL string, token string, profile device.Profile, interval time.Duration, stop <-chan struct{}) error {
		close(started)
		<-stop
		close(stopped)
		return nil
	}

	if err := service.StartHeartbeat(); err != nil {
		t.Fatalf("StartHeartbeat() error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat worker did not start")
	}

	done := make(chan struct{})
	go func() {
		service.StopHeartbeat()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StopHeartbeat() did not return promptly")
	}

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("heartbeat worker did not observe stop signal")
	}
}

func TestStopHeartbeatReturnsEvenIfWorkerIgnoresStop(t *testing.T) {
	root := t.TempDir()

	cfg := clientconfig.Default()
	cfg.ServerURL = "http://example.com"
	cfg.Token = "token"
	cfg.Device = clientconfig.DeviceConfig{
		DeviceID:      "device-1",
		DeviceName:    "demo-device",
		DeviceType:    "windows",
		ClientVersion: "desktop-0.1.0",
	}
	if err := clientconfig.EnsureRoot(root); err != nil {
		t.Fatalf("EnsureRoot() error = %v", err)
	}
	if err := clientconfig.Save(root, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := device.Save(root, device.Profile{
		DeviceID:      cfg.Device.DeviceID,
		DeviceName:    cfg.Device.DeviceName,
		DeviceType:    cfg.Device.DeviceType,
		ClientVersion: cfg.Device.ClientVersion,
	}); err != nil {
		t.Fatalf("device.Save() error = %v", err)
	}

	service, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	originalStopWait := heartbeatStopWait
	heartbeatStopWait = 20 * time.Millisecond
	defer func() {
		heartbeatStopWait = originalStopWait
	}()

	started := make(chan struct{})
	release := make(chan struct{})
	service.heartbeatFn = func(serverURL string, token string, profile device.Profile, interval time.Duration, stop <-chan struct{}) error {
		close(started)
		<-release
		return nil
	}

	if err := service.StartHeartbeat(); err != nil {
		t.Fatalf("StartHeartbeat() error = %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat worker did not start")
	}

	begin := time.Now()
	service.StopHeartbeat()
	if elapsed := time.Since(begin); elapsed > 250*time.Millisecond {
		t.Fatalf("StopHeartbeat() took too long: %v", elapsed)
	}

	close(release)
}
