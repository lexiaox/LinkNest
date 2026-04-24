package appsvc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"linknest/client/internal/auth"
	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/device"
	"linknest/client/internal/transfer"
	clientws "linknest/client/internal/websocket"
)

const desktopClientVersion = "desktop-0.1.0"

var registerDevice = device.Register
var heartbeatStopWait = 2 * time.Second

type HeartbeatFunc func(serverURL string, token string, profile device.Profile, interval time.Duration, stop <-chan struct{}) error

type Snapshot struct {
	ServerURL        string
	HasToken         bool
	DeviceID         string
	DeviceName       string
	DeviceType       string
	ClientVersion    string
	HeartbeatRunning bool
	HeartbeatError   string
}

type Service struct {
	root            string
	mu              sync.Mutex
	cfg             clientconfig.ClientConfig
	heartbeatFn     HeartbeatFunc
	heartbeatStopCh chan struct{}
	heartbeatDoneCh chan struct{}
	heartbeatErr    string
}

func New(root string) (*Service, error) {
	if err := clientconfig.EnsureRoot(root); err != nil {
		return nil, err
	}

	cfg, err := clientconfig.Load(root)
	if err != nil {
		return nil, err
	}

	return &Service{
		root:        root,
		cfg:         cfg,
		heartbeatFn: clientws.RunHeartbeatUntil,
	}, nil
}

func (s *Service) Root() string {
	return s.root
}

func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Snapshot{
		ServerURL:        strings.TrimSpace(s.cfg.ServerURL),
		HasToken:         strings.TrimSpace(s.cfg.Token) != "",
		DeviceID:         s.cfg.Device.DeviceID,
		DeviceName:       s.cfg.Device.DeviceName,
		DeviceType:       s.cfg.Device.DeviceType,
		ClientVersion:    s.cfg.Device.ClientVersion,
		HeartbeatRunning: s.heartbeatStopCh != nil,
		HeartbeatError:   s.heartbeatErr,
	}
}

func (s *Service) SetServerURL(serverURL string) error {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return errors.New("server URL is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cfg.ServerURL = serverURL
	return s.saveLocked()
}

func (s *Service) Register(username string, email string, password string) (auth.AuthResult, error) {
	client := auth.NewClient(s.serverURL())
	result, err := client.Register(auth.RegisterInput{
		Username: strings.TrimSpace(username),
		Email:    strings.TrimSpace(email),
		Password: strings.TrimSpace(password),
	})
	if err != nil {
		return auth.AuthResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Token = result.Token
	if err := s.saveLocked(); err != nil {
		return auth.AuthResult{}, err
	}
	return result, nil
}

func (s *Service) Login(username string, password string) (auth.AuthResult, error) {
	client := auth.NewClient(s.serverURL())
	result, err := client.Login(auth.LoginInput{
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
	})
	if err != nil {
		return auth.AuthResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Token = result.Token
	if err := s.saveLocked(); err != nil {
		return auth.AuthResult{}, err
	}
	return result, nil
}

func (s *Service) DeleteAccount(password string) (auth.DeleteAccountResult, error) {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return auth.DeleteAccountResult{}, err
	}

	result, err := auth.NewClient(serverURL).DeleteAccount(token, auth.DeleteAccountInput{
		Password: strings.TrimSpace(password),
	})
	if err != nil {
		return auth.DeleteAccountResult{}, err
	}

	s.StopHeartbeat()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Token = ""
	if err := s.saveLocked(); err != nil {
		return auth.DeleteAccountResult{}, err
	}
	return result, nil
}

func (s *Service) BindCurrentDevice(deviceName string, deviceType string) (device.Profile, error) {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return device.Profile{}, err
	}

	profile, err := device.Init(s.root, deviceName, deviceType, desktopClientVersion)
	if err != nil {
		return device.Profile{}, err
	}
	if err := registerDevice(serverURL, token, profile); err != nil {
		return device.Profile{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Device = clientconfig.DeviceConfig{
		DeviceID:      profile.DeviceID,
		DeviceName:    profile.DeviceName,
		DeviceType:    profile.DeviceType,
		ClientVersion: profile.ClientVersion,
	}
	if err := s.saveLocked(); err != nil {
		return device.Profile{}, err
	}
	return profile, nil
}

func (s *Service) ListDevices() ([]device.RemoteDevice, error) {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return nil, err
	}
	return device.List(serverURL, token)
}

func (s *Service) ListFiles() ([]transfer.RemoteFile, error) {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return nil, err
	}
	return transfer.ListFiles(serverURL, token)
}

func (s *Service) ListTasks() ([]transfer.RemoteTask, error) {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return nil, err
	}
	return transfer.ListTasks(serverURL, token)
}

func (s *Service) Upload(localPath string) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}
	return transfer.Upload(s.root, cfg, localPath)
}

func (s *Service) Download(fileID string, output string) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}
	return transfer.Download(s.root, cfg, strings.TrimSpace(fileID), output)
}

func (s *Service) DeleteFile(fileID string) error {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return err
	}
	return transfer.DeleteFile(serverURL, token, strings.TrimSpace(fileID))
}

func (s *Service) ResumeTask(uploadID string) error {
	cfg, err := s.loadConfig()
	if err != nil {
		return err
	}
	return transfer.Resume(s.root, cfg, strings.TrimSpace(uploadID))
}

func (s *Service) StartHeartbeat() error {
	serverURL, token, err := s.requireToken()
	if err != nil {
		return err
	}

	profile, err := device.Load(s.root)
	if err != nil {
		return fmt.Errorf("load device profile: %w", err)
	}

	s.mu.Lock()
	if s.heartbeatStopCh != nil {
		s.mu.Unlock()
		return errors.New("heartbeat is already running")
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	s.heartbeatStopCh = stopCh
	s.heartbeatDoneCh = doneCh
	s.heartbeatErr = ""
	s.mu.Unlock()

	go func() {
		defer close(doneCh)
		for {
			select {
			case <-stopCh:
				return
			default:
			}

			err := s.heartbeatFn(serverURL, token, profile, 5*time.Second, stopCh)
			if err != nil {
				s.mu.Lock()
				s.heartbeatErr = err.Error()
				s.mu.Unlock()
			}

			select {
			case <-stopCh:
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()

	return nil
}

func (s *Service) StopHeartbeat() {
	s.mu.Lock()
	stopCh := s.heartbeatStopCh
	doneCh := s.heartbeatDoneCh
	s.heartbeatStopCh = nil
	s.heartbeatDoneCh = nil
	s.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(heartbeatStopWait):
		}
	}
}

func (s *Service) loadConfig() (clientconfig.ClientConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := clientconfig.Load(s.root)
	if err != nil {
		return clientconfig.ClientConfig{}, err
	}
	s.cfg = cfg
	return s.cfg, nil
}

func (s *Service) requireToken() (string, string, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return "", "", err
	}

	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		return "", "", errors.New("server URL is empty")
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return "", "", errors.New("token is empty, please login first")
	}
	return serverURL, token, nil
}

func (s *Service) serverURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.cfg.ServerURL)
}

func (s *Service) saveLocked() error {
	return clientconfig.Save(s.root, s.cfg)
}
