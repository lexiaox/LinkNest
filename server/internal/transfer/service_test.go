package transfer

import (
	"context"
	"database/sql"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"linknest/server/internal/device"
	"linknest/server/internal/p2ptoken"

	_ "github.com/mattn/go-sqlite3"
)

func TestInitPrefersP2PWhenTargetIsOnlineAndCapable(t *testing.T) {
	service, deviceService := newTestService(t)
	ctx := context.Background()
	registerTransferDevice(t, ctx, deviceService, 1, "source")
	registerTransferDevice(t, ctx, deviceService, 1, "target")
	if err := deviceService.UpdateHeartbeat(ctx, 1, device.Heartbeat{
		DeviceID:    "target",
		LanIP:       "192.168.1.20",
		P2PEnabled:  true,
		P2PPort:     19090,
		P2PProtocol: "http",
	}); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}

	result, err := service.Init(ctx, 1, InitInput{
		SourceDeviceID: "source",
		TargetDeviceID: "target",
		FileName:       "demo.bin",
		FileSize:       12,
		FileHash:       "hash",
		ChunkSize:      4,
		TotalChunks:    3,
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if result.PreferredRoute != RouteP2P || result.Status != StatusInitialized {
		t.Fatalf("Init() route/status = %s/%s, want p2p/initialized", result.PreferredRoute, result.Status)
	}
	if len(result.P2PCandidates) != 1 || result.P2PCandidates[0].Host != "192.168.1.20" {
		t.Fatalf("P2PCandidates = %#v, want LAN candidate", result.P2PCandidates)
	}
}

func TestInitFallsBackToCloudWhenTargetIsOffline(t *testing.T) {
	service, deviceService := newTestService(t)
	ctx := context.Background()
	registerTransferDevice(t, ctx, deviceService, 1, "source")
	registerTransferDevice(t, ctx, deviceService, 1, "target")

	result, err := service.Init(ctx, 1, InitInput{
		SourceDeviceID: "source",
		TargetDeviceID: "target",
		FileName:       "demo.bin",
		FileSize:       12,
		FileHash:       "hash",
		ChunkSize:      4,
		TotalChunks:    3,
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if result.PreferredRoute != RouteCloud || result.Status != StatusFallbackUploading {
		t.Fatalf("Init() route/status = %s/%s, want cloud/fallback_uploading", result.PreferredRoute, result.Status)
	}
	if len(result.P2PCandidates) != 0 {
		t.Fatalf("P2PCandidates = %#v, want empty candidates", result.P2PCandidates)
	}
}

func TestValidateTokenRejectsWrongTargetDevice(t *testing.T) {
	service, deviceService := newTestService(t)
	ctx := context.Background()
	registerTransferDevice(t, ctx, deviceService, 1, "source")
	registerTransferDevice(t, ctx, deviceService, 1, "target")

	result, err := service.Init(ctx, 1, InitInput{
		SourceDeviceID: "source",
		TargetDeviceID: "target",
		FileName:       "demo.bin",
		FileSize:       12,
		FileHash:       "hash",
		ChunkSize:      4,
		TotalChunks:    3,
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, err = service.ValidateToken(ctx, 1, ValidateTokenInput{
		TransferToken: result.TransferToken,
		TransferID:    result.TransferID,
		DeviceID:      "source",
	})
	if err != ErrInvalidToken {
		t.Fatalf("ValidateToken() error = %v, want ErrInvalidToken", err)
	}
}

func newTestService(t *testing.T) (*Service, *device.Service) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, migration := range []string{"001_init.sql", "002_v2_p2p_transfers.sql"} {
		raw, err := ioutil.ReadFile(filepath.Join("..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if _, err := db.Exec(string(raw)); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO users (id, username, email, password_hash) VALUES (1, 'u1', 'u1@example.com', 'hash')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	deviceService := device.NewService(db)
	tokenService := p2ptoken.New([]byte("secret"), time.Minute)
	return NewService(db, deviceService, tokenService), deviceService
}

func registerTransferDevice(t *testing.T, ctx context.Context, service *device.Service, userID int64, deviceID string) {
	t.Helper()
	if _, err := service.RegisterOrUpdate(ctx, userID, device.RegisterInput{
		DeviceID:   deviceID,
		DeviceName: deviceID,
		DeviceType: "test",
	}); err != nil {
		t.Fatalf("RegisterOrUpdate(%s) error = %v", deviceID, err)
	}
}
