package device

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"linknest/client/internal/httpx"

	"github.com/google/uuid"
)

type Profile struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type"`
	ClientVersion string `json:"client_version"`
}

type RemoteDevice struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	DeviceType string `json:"device_type"`
	Status     string `json:"status"`
	LastSeenAt string `json:"last_seen_at"`
}

type deviceListResponse struct {
	Items []RemoteDevice `json:"items"`
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func Init(root string, deviceName string, deviceType string, version string) (Profile, error) {
	deviceName = strings.TrimSpace(deviceName)
	deviceType = strings.TrimSpace(deviceType)
	version = strings.TrimSpace(version)

	if deviceName == "" {
		deviceName = DefaultDeviceName()
	}
	if deviceType == "" {
		deviceType = DefaultDeviceType()
	}
	if version == "" {
		version = "0.1.0"
	}

	if profile, err := Load(root); err == nil {
		if deviceName != "" {
			profile.DeviceName = deviceName
		} else if profile.DeviceName == "" {
			profile.DeviceName = deviceName
		}
		if deviceType != "" {
			profile.DeviceType = deviceType
		} else if profile.DeviceType == "" {
			profile.DeviceType = deviceType
		}
		if version != "" {
			profile.ClientVersion = version
		} else if profile.ClientVersion == "" {
			profile.ClientVersion = version
		}
		if err := Save(root, profile); err != nil {
			return Profile{}, err
		}
		return profile, nil
	}

	profile := Profile{
		DeviceID:      uuid.New().String(),
		DeviceName:    deviceName,
		DeviceType:    deviceType,
		ClientVersion: version,
	}

	if err := Save(root, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func DefaultDeviceName() string {
	hostname, err := os.Hostname()
	if err == nil {
		hostname = strings.TrimSpace(hostname)
	}
	if hostname == "" {
		return "linknest-device"
	}
	return hostname
}

func DefaultDeviceType() string {
	deviceType := strings.TrimSpace(runtime.GOOS)
	if deviceType == "" {
		return "unknown"
	}
	return deviceType
}

func Load(root string) (Profile, error) {
	raw, err := ioutil.ReadFile(path(root))
	if err != nil {
		return Profile{}, err
	}

	var profile Profile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return Profile{}, err
	}
	if strings.TrimSpace(profile.DeviceID) == "" {
		return Profile{}, errors.New("device_id is empty")
	}
	return profile, nil
}

func Save(root string, profile Profile) error {
	raw, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path(root), raw, 0644)
}

func Register(baseURL string, token string, profile Profile) error {
	payload, err := json.Marshal(profile)
	if err != nil {
		return err
	}

	resp, err := httpx.Do(httpx.NewClient(20*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/devices/register", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return parseError(resp)
}

func List(baseURL string, token string) ([]RemoteDevice, error) {
	resp, err := httpx.Do(httpx.NewClient(20*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/devices", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, decodeError(body, resp.Status)
	}

	var result deviceListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func DetectLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func HeartbeatPayload(profile Profile) map[string]interface{} {
	return map[string]interface{}{
		"type":           "heartbeat",
		"device_id":      profile.DeviceID,
		"device_name":    profile.DeviceName,
		"device_type":    profile.DeviceType,
		"lan_ip":         DetectLANIP(),
		"port":           0,
		"client_version": profile.ClientVersion,
		"timestamp":      time.Now().Format(time.RFC3339),
	}
}

func path(root string) string {
	return filepath.Join(root, "device.json")
}

func parseError(resp *http.Response) error {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return decodeError(body, resp.Status)
	}
	return nil
}

func decodeError(body []byte, status string) error {
	var remoteErr errorBody
	if err := json.Unmarshal(body, &remoteErr); err == nil && remoteErr.Error.Message != "" {
		return fmt.Errorf("%s: %s", remoteErr.Error.Code, remoteErr.Error.Message)
	}
	return fmt.Errorf("request failed: %s", status)
}
