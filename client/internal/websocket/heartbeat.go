package websocket

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"linknest/client/internal/device"

	gws "github.com/gorilla/websocket"
)

func RunHeartbeat(serverURL string, token string, profile device.Profile, interval time.Duration) error {
	return RunHeartbeatUntil(serverURL, token, profile, interval, nil)
}

func RunHeartbeatUntil(serverURL string, token string, profile device.Profile, interval time.Duration, stop <-chan struct{}) error {
	wsURL, err := toWebSocketURL(strings.TrimRight(serverURL, "/"), profile.DeviceID)
	if err != nil {
		return err
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := &gws.Dialer{
		Proxy:            nil,
		HandshakeTimeout: 10 * time.Second,
		NetDialContext:   (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return err
	}
	defer conn.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	writeHeartbeat := func() error {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(device.HeartbeatPayload(profile))
	}

	if err := writeHeartbeat(); err != nil {
		return err
	}

	readErrCh := make(chan error, 1)
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				readErrCh <- err
				return
			}
		}
	}()

	for {
		select {
		case <-stop:
			_ = conn.Close()
			return nil
		case err := <-readErrCh:
			return err
		case <-ticker.C:
			if err := writeHeartbeat(); err != nil {
				return err
			}
		}
	}
}

func toWebSocketURL(serverURL string, deviceID string) (string, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported server url scheme: %s", parsed.Scheme)
	}

	parsed.Path = "/ws/devices"
	query := parsed.Query()
	query.Set("device_id", deviceID)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
