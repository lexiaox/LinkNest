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
	ChunkSize int64 `yaml:"chunk_size"`
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
			ChunkSize: 4194304,
		},
	}
}
