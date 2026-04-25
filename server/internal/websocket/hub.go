package websocket

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"linknest/server/internal/device"
	"linknest/server/internal/middleware"
	"linknest/server/internal/response"

	gws "github.com/gorilla/websocket"
)

type Handler struct {
	device   *device.Service
	upgrader gws.Upgrader
}

type heartbeatMessage struct {
	Type          string `json:"type"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type"`
	LanIP         string `json:"lan_ip"`
	Port          int    `json:"port"`
	P2PEnabled    bool   `json:"p2p_enabled"`
	P2PPort       int    `json:"p2p_port"`
	P2PProtocol   string `json:"p2p_protocol"`
	VirtualIP     string `json:"virtual_ip"`
	PublicIP      string `json:"public_ip"`
	ClientVersion string `json:"client_version"`
	Timestamp     string `json:"timestamp"`
}

func NewHandler(deviceService *device.Service) *Handler {
	return &Handler{
		device: deviceService,
		upgrader: gws.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
		return
	}

	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		response.Error(w, http.StatusBadRequest, "DEVICE_NOT_FOUND", "device_id is required")
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		var msg heartbeatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}

		if strings.TrimSpace(msg.DeviceID) == "" {
			msg.DeviceID = deviceID
		}
		if msg.DeviceID != deviceID {
			conn.WriteJSON(map[string]string{
				"type":    "error",
				"code":    "DEVICE_NOT_FOUND",
				"message": "device_id mismatch",
			})
			continue
		}
		if strings.TrimSpace(msg.Type) != "" && msg.Type != "heartbeat" {
			conn.WriteJSON(map[string]string{
				"type":    "error",
				"code":    "BAD_REQUEST",
				"message": "unsupported websocket message type",
			})
			continue
		}

		err := h.device.UpdateHeartbeat(r.Context(), user.ID, device.Heartbeat{
			DeviceID:      msg.DeviceID,
			DeviceName:    msg.DeviceName,
			DeviceType:    msg.DeviceType,
			LanIP:         msg.LanIP,
			Port:          msg.Port,
			P2PEnabled:    msg.P2PEnabled,
			P2PPort:       msg.P2PPort,
			P2PProtocol:   msg.P2PProtocol,
			VirtualIP:     msg.VirtualIP,
			PublicIP:      msg.PublicIP,
			ClientVersion: msg.ClientVersion,
			Timestamp:     msg.Timestamp,
		})
		if err != nil {
			code := "INTERNAL_ERROR"
			message := err.Error()
			if errors.Is(err, device.ErrDeviceNotFound) {
				code = "DEVICE_NOT_FOUND"
				message = "device does not exist or does not belong to current user"
			}
			conn.WriteJSON(map[string]string{
				"type":    "error",
				"code":    code,
				"message": message,
			})
			continue
		}

		conn.WriteJSON(map[string]string{
			"type":        "heartbeat_ack",
			"server_time": time.Now().Format(time.RFC3339),
			"status":      "online",
		})
	}
}
