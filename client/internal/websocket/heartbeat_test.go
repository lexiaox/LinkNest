package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"linknest/client/internal/device"

	gws "github.com/gorilla/websocket"
)

func TestRunHeartbeatUntilStopsHealthyConnection(t *testing.T) {
	upgrader := gws.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	receivedHeartbeat := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read heartbeat error: %v", err)
			return
		}
		close(receivedHeartbeat)

		select {}
	}))
	defer server.Close()

	stop := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- RunHeartbeatUntil(server.URL, "token", device.Profile{
			DeviceID:      "device-1",
			DeviceName:    "desktop-test",
			DeviceType:    "windows",
			ClientVersion: "desktop-0.1.0",
		}, time.Hour, stop)
	}()

	select {
	case <-receivedHeartbeat:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive initial heartbeat")
	}

	close(stop)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunHeartbeatUntil() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunHeartbeatUntil() did not stop after stop signal")
	}
}
