package appsvc

import (
	"testing"

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

	service, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
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
