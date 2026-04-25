package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ClientConfig struct {
	ServerURL string         `yaml:"server_url"`
	Token     string         `yaml:"token"`
	Device    DeviceConfig   `yaml:"device"`
	Transfer  TransferConfig `yaml:"transfer"`
}

type DeviceConfig struct {
	DeviceID      string `yaml:"device_id"`
	DeviceName    string `yaml:"device_name"`
	DeviceType    string `yaml:"device_type"`
	ClientVersion string `yaml:"client_version"`
}

type TransferConfig struct {
	ChunkSize                int64  `yaml:"chunk_size"`
	P2PEnabled               *bool  `yaml:"p2p_enabled"`
	P2PHost                  string `yaml:"p2p_host"`
	P2PPort                  int    `yaml:"p2p_port"`
	P2PConnectTimeoutSeconds int    `yaml:"p2p_connect_timeout_seconds"`
	P2PChunkTimeoutSeconds   int    `yaml:"p2p_chunk_timeout_seconds"`
	P2PMaxRetries            int    `yaml:"p2p_max_retries"`
	FallbackToCloud          *bool  `yaml:"fallback_to_cloud"`
	InboxDir                 string `yaml:"inbox_dir"`
	VirtualIP                string `yaml:"virtual_ip"`
}

func RootDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".linknest"), nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	return filepath.Join(currentUser.HomeDir, ".linknest"), nil
}

func EnsureRoot(root string) error {
	dirs := []string{
		root,
		filepath.Join(root, "tasks"),
		filepath.Join(root, "transfers"),
		filepath.Join(root, "inbox"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func ConfigPath(root string) string {
	return filepath.Join(root, "config.yaml")
}

func Load(root string) (ClientConfig, error) {
	path := ConfigPath(root)
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return ClientConfig{}, err
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ClientConfig{}, err
	}

	if strings.TrimSpace(cfg.ServerURL) == "" {
		cfg.ServerURL = "http://127.0.0.1:8080"
	}
	if cfg.Transfer.ChunkSize <= 0 {
		cfg.Transfer.ChunkSize = 4194304
	}
	normalizeTransferConfig(root, &cfg.Transfer)
	return cfg, nil
}

func Save(root string, cfg ClientConfig) error {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(ConfigPath(root), raw, 0644)
}

func Default() ClientConfig {
	return ClientConfig{
		ServerURL: "http://127.0.0.1:8080",
		Transfer: TransferConfig{
			ChunkSize:                4194304,
			P2PEnabled:               boolPtr(true),
			P2PHost:                  "0.0.0.0",
			P2PPort:                  19090,
			P2PConnectTimeoutSeconds: 5,
			P2PChunkTimeoutSeconds:   30,
			P2PMaxRetries:            2,
			FallbackToCloud:          boolPtr(true),
		},
	}
}

func normalizeTransferConfig(root string, cfg *TransferConfig) {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 4194304
	}
	if strings.TrimSpace(cfg.P2PHost) == "" {
		cfg.P2PHost = "0.0.0.0"
	}
	if cfg.P2PPort <= 0 {
		cfg.P2PPort = 19090
	}
	if cfg.P2PConnectTimeoutSeconds <= 0 {
		cfg.P2PConnectTimeoutSeconds = 5
	}
	if cfg.P2PChunkTimeoutSeconds <= 0 {
		cfg.P2PChunkTimeoutSeconds = 30
	}
	if cfg.P2PMaxRetries < 0 {
		cfg.P2PMaxRetries = 0
	}
	if strings.TrimSpace(cfg.InboxDir) == "" {
		cfg.InboxDir = filepath.Join(root, "inbox")
	}
	if cfg.P2PEnabled == nil {
		cfg.P2PEnabled = boolPtr(true)
	}
	if cfg.FallbackToCloud == nil {
		cfg.FallbackToCloud = boolPtr(true)
	}
}

func P2PEnabledValue(cfg TransferConfig) bool {
	return cfg.P2PEnabled == nil || *cfg.P2PEnabled
}

func FallbackToCloudEnabled(cfg TransferConfig) bool {
	return cfg.FallbackToCloud == nil || *cfg.FallbackToCloud
}

func boolPtr(value bool) *bool {
	return &value
}
