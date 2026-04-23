package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
	Transfer TransferConfig `yaml:"transfer"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type AuthConfig struct {
	JWTSecret     string `yaml:"jwt_secret"`
	TokenTTLHours int    `yaml:"token_ttl_hours"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type StorageConfig struct {
	Type     string `yaml:"type"`
	RootDir  string `yaml:"root_dir"`
	ChunkDir string `yaml:"chunk_dir"`
}

type TransferConfig struct {
	ChunkSize                int64 `yaml:"chunk_size"`
	HeartbeatIntervalSeconds int   `yaml:"heartbeat_interval_seconds"`
	OfflineTimeoutSeconds    int   `yaml:"offline_timeout_seconds"`
}

func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, errors.New("config path is required")
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(raw))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Server.Host) == "" {
		return errors.New("server.host is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return errors.New("auth.jwt_secret is required; set LINKNEST_JWT_SECRET")
	}
	if c.Auth.TokenTTLHours <= 0 {
		return errors.New("auth.token_ttl_hours must be greater than 0")
	}
	if strings.TrimSpace(c.Database.Driver) != "sqlite" {
		return errors.New("database.driver must be sqlite")
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return errors.New("database.dsn is required")
	}
	if strings.TrimSpace(c.Storage.Type) == "" {
		c.Storage.Type = "local"
	}
	if c.Storage.Type != "local" {
		return errors.New("storage.type must be local in V1")
	}
	if strings.TrimSpace(c.Storage.RootDir) == "" {
		return errors.New("storage.root_dir is required")
	}
	if strings.TrimSpace(c.Storage.ChunkDir) == "" {
		return errors.New("storage.chunk_dir is required")
	}
	if c.Transfer.ChunkSize <= 0 {
		return errors.New("transfer.chunk_size must be greater than 0")
	}
	if c.Transfer.HeartbeatIntervalSeconds <= 0 {
		return errors.New("transfer.heartbeat_interval_seconds must be greater than 0")
	}
	if c.Transfer.OfflineTimeoutSeconds <= 0 {
		return errors.New("transfer.offline_timeout_seconds must be greater than 0")
	}
	return nil
}

func (c ServerConfig) Addr() string {
	return net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
}

func (c AuthConfig) TokenTTL() time.Duration {
	return time.Duration(c.TokenTTLHours) * time.Hour
}

func (c TransferConfig) HeartbeatInterval() time.Duration {
	return time.Duration(c.HeartbeatIntervalSeconds) * time.Second
}

func (c TransferConfig) OfflineTimeout() time.Duration {
	return time.Duration(c.OfflineTimeoutSeconds) * time.Second
}
