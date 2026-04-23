package websocket

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"linknest/client/internal/device"

	gws "github.com/gorilla/websocket"
)

func RunHeartbeat(serverURL string, token string, profile device.Profile, interval time.Duration) error {
	wsURL, err := toWebSocketURL(strings.TrimRight(serverURL, "/"), profile.DeviceID)
	if err != nil {
		return err
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	conn, _, err := gws.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		return err
	}
	defer conn.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := conn.WriteJSON(device.HeartbeatPayload(profile)); err != nil {
		return err
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return err
		}

		<-ticker.C
		if err := conn.WriteJSON(device.HeartbeatPayload(profile)); err != nil {
			return err
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
