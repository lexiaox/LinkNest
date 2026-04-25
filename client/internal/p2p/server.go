package p2p

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/device"
	"linknest/client/internal/httpx"
)

type Server struct {
	root       string
	cfg        clientconfig.ClientConfig
	profile    device.Profile
	server     *http.Server
	listener   net.Listener
	actualPort int
	mu         sync.Mutex
	cache      map[string]cacheEntry
}

type TokenMetadata struct {
	TransferID     string `json:"transfer_id"`
	SourceDeviceID string `json:"source_device_id"`
	TargetDeviceID string `json:"target_device_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	Valid          bool   `json:"valid"`
}

type cacheEntry struct {
	meta      TokenMetadata
	expiresAt time.Time
}

type LocalTask struct {
	TransferID     string `json:"transfer_id"`
	SourceDeviceID string `json:"source_device_id"`
	TargetDeviceID string `json:"target_device_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	ReceivedChunks []int  `json:"received_chunks"`
	Status         string `json:"status"`
	OutputPath     string `json:"output_path,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

type probeRequest struct {
	TransferID     string `json:"transfer_id"`
	SourceDeviceID string `json:"source_device_id"`
	FileHash       string `json:"file_hash"`
}

type completeRequest struct {
	FileHash    string `json:"file_hash"`
	TotalChunks int    `json:"total_chunks"`
}

type validateTokenRequest struct {
	TransferToken string `json:"transfer_token"`
	TransferID    string `json:"transfer_id"`
	DeviceID      string `json:"device_id"`
}

type completeTransferRequest struct {
	Route          string `json:"route"`
	FileHash       string `json:"file_hash"`
	ReceivedChunks int    `json:"received_chunks"`
}

func Start(root string, cfg clientconfig.ClientConfig, profile device.Profile) (*Server, error) {
	host := strings.TrimSpace(cfg.Transfer.P2PHost)
	if host == "" {
		host = "0.0.0.0"
	}
	startPort := cfg.Transfer.P2PPort
	if startPort <= 0 {
		startPort = 19090
	}

	var listener net.Listener
	var actualPort int
	var lastErr error
	for port := startPort; port < startPort+10; port++ {
		candidate, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			lastErr = err
			continue
		}
		listener = candidate
		actualPort = port
		break
	}
	if listener == nil {
		return nil, fmt.Errorf("listen p2p service: %w", lastErr)
	}

	s := &Server{
		root:       root,
		cfg:        cfg,
		profile:    profile,
		listener:   listener,
		actualPort: actualPort,
		cache:      make(map[string]cacheEntry),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/p2p/v1/probe", s.handleProbe)
	mux.HandleFunc("/p2p/v1/transfers/", s.handleTransferRoutes)
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.Transfer.P2PChunkTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(cfg.Transfer.P2PChunkTimeoutSeconds) * time.Second,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "p2p service stopped: %v\n", err)
		}
	}()
	return s, nil
}

func (s *Server) Port() int {
	return s.actualPort
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input probeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta, err := s.validateRequest(r, input.TransferID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if meta.SourceDeviceID != strings.TrimSpace(input.SourceDeviceID) || !strings.EqualFold(meta.FileHash, strings.TrimSpace(input.FileHash)) {
		http.Error(w, "probe metadata mismatch", http.StatusUnauthorized)
		return
	}
	if err := s.saveTask(meta, nil, "probing", ""); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"device_id": s.profile.DeviceID,
		"ready":     true,
	})
}

func (s *Server) handleTransferRoutes(w http.ResponseWriter, r *http.Request) {
	transferID, action, ok := parseP2PTransferPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodPut && strings.HasPrefix(action, "chunks/"):
		chunkIndex, err := strconv.Atoi(strings.TrimPrefix(action, "chunks/"))
		if err != nil {
			http.Error(w, "chunk index must be an integer", http.StatusBadRequest)
			return
		}
		s.handleChunk(w, r, transferID, chunkIndex)
	case r.Method == http.MethodPost && action == "complete":
		s.handleComplete(w, r, transferID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleChunk(w http.ResponseWriter, r *http.Request, transferID string, chunkIndex int) {
	meta, err := s.validateRequest(r, transferID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if chunkIndex < 0 || chunkIndex >= meta.TotalChunks {
		http.Error(w, "chunk index out of range", http.StatusBadRequest)
		return
	}

	// Limit the body to the declared chunk size plus a small overhead for
	// protocol framing and headers; this prevents a malicious sender from
	// exhausting receiver memory with an oversized payload.
	r.Body = http.MaxBytesReader(w, r.Body, int64(meta.ChunkSize)+1024)
	raw, err := ioutil.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	actualHash := hashBytes(raw)
	expectedHash := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Chunk-Hash")))
	if expectedHash == "" || actualHash != expectedHash {
		http.Error(w, "chunk hash mismatch", http.StatusConflict)
		return
	}

	chunkPath := s.chunkPath(transferID, chunkIndex)
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := chunkPath + ".tmp"
	if err := ioutil.WriteFile(tmpPath, raw, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpPath, chunkPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	received := appendUnique(s.receivedChunks(transferID), chunkIndex)
	if err := s.saveTask(meta, received, "transferring", ""); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"transfer_id":     transferID,
		"chunk_index":     chunkIndex,
		"received_chunks": len(received),
		"total_chunks":    meta.TotalChunks,
	})
}

func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request, transferID string) {
	var input completeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta, err := s.validateRequest(r, transferID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	totalChunks := input.TotalChunks
	if totalChunks <= 0 {
		totalChunks = meta.TotalChunks
	}
	outputPath, actualHash, err := s.mergeChunks(meta, totalChunks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	expectedHash := strings.ToLower(strings.TrimSpace(input.FileHash))
	if expectedHash == "" {
		expectedHash = strings.ToLower(strings.TrimSpace(meta.FileHash))
	}
	if actualHash != expectedHash {
		_ = os.Remove(outputPath)
		http.Error(w, "file hash mismatch", http.StatusConflict)
		return
	}

	received := s.receivedChunks(transferID)
	if err := s.saveTask(meta, received, "completed", outputPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.reportComplete(meta, len(received))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"transfer_id":     transferID,
		"output":          outputPath,
		"received_chunks": len(received),
		"file_hash":       actualHash,
	})
}

func (s *Server) validateRequest(r *http.Request, transferID string) (TokenMetadata, error) {
	token, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		return TokenMetadata{}, err
	}
	cacheKey := transferID + ":" + token

	s.mu.Lock()
	if entry, ok := s.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		s.mu.Unlock()
		return entry.meta, nil
	}
	delete(s.cache, cacheKey)
	s.mu.Unlock()

	payload := validateTokenRequest{
		TransferToken: token,
		TransferID:    transferID,
		DeviceID:      s.profile.DeviceID,
	}
	var result TokenMetadata
	if err := s.doJSON(http.MethodPost, "/api/transfers/validate-token", payload, &result); err != nil {
		return TokenMetadata{}, err
	}
	if !result.Valid || result.TargetDeviceID != s.profile.DeviceID {
		return TokenMetadata{}, errors.New("transfer token target mismatch")
	}

	// Default to 1 hour when the token does not carry an expiry or it cannot
	// be parsed; this bounds cache growth without relying on token contents.
	expiresAt := time.Now().Add(time.Hour)
	if result.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, result.ExpiresAt); err == nil {
			expiresAt = t
		}
	}

	s.mu.Lock()
	s.cache[cacheKey] = cacheEntry{meta: result, expiresAt: expiresAt}
	s.mu.Unlock()
	return result, nil
}

func (s *Server) reportComplete(meta TokenMetadata, receivedChunks int) error {
	payload := completeTransferRequest{
		Route:          "p2p",
		FileHash:       meta.FileHash,
		ReceivedChunks: receivedChunks,
	}
	var result map[string]interface{}
	return s.doJSON(http.MethodPost, "/api/transfers/"+meta.TransferID+"/complete", payload, &result)
}

func (s *Server) doJSON(method string, path string, payload interface{}, out interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := httpx.Do(httpx.NewClient(15*time.Second), 1, func() (*http.Request, error) {
		req, err := http.NewRequest(method, strings.TrimRight(s.cfg.ServerURL, "/")+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (s *Server) saveTask(meta TokenMetadata, received []int, status string, output string) error {
	if received == nil {
		received = s.receivedChunks(meta.TransferID)
	}
	task := LocalTask{
		TransferID:     meta.TransferID,
		SourceDeviceID: meta.SourceDeviceID,
		TargetDeviceID: meta.TargetDeviceID,
		FileName:       meta.FileName,
		FileSize:       meta.FileSize,
		FileHash:       meta.FileHash,
		ChunkSize:      meta.ChunkSize,
		TotalChunks:    meta.TotalChunks,
		ReceivedChunks: received,
		Status:         status,
		OutputPath:     output,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.transferDir(meta.TransferID), 0755); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return ioutil.WriteFile(s.taskPath(meta.TransferID), raw, 0644)
}

func (s *Server) receivedChunks(transferID string) []int {
	entries, err := ioutil.ReadDir(filepath.Join(s.transferDir(transferID), "chunks"))
	if err != nil {
		return nil
	}
	var chunks []int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".part"))
		if err == nil {
			chunks = append(chunks, index)
		}
	}
	return chunks
}

func (s *Server) mergeChunks(meta TokenMetadata, totalChunks int) (string, string, error) {
	finalDir := filepath.Join(s.transferDir(meta.TransferID), "final")
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		return "", "", err
	}
	outputPath := filepath.Join(finalDir, safeFileName(meta.FileName))
	if _, err := os.Stat(outputPath); err == nil {
		outputPath = filepath.Join(finalDir, fmt.Sprintf("%s-%d", safeFileName(meta.FileName), time.Now().UnixNano()))
	}
	output, err := os.Create(outputPath)
	if err != nil {
		return "", "", err
	}
	defer output.Close()

	hasher := sha256.New()
	for i := 0; i < totalChunks; i++ {
		input, err := os.Open(s.chunkPath(meta.TransferID, i))
		if err != nil {
			return "", "", fmt.Errorf("missing chunk %d", i)
		}
		if _, err := io.Copy(io.MultiWriter(output, hasher), input); err != nil {
			input.Close()
			return "", "", err
		}
		if err := input.Close(); err != nil {
			return "", "", err
		}
	}
	if err := output.Close(); err != nil {
		return "", "", err
	}
	return outputPath, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *Server) transferDir(transferID string) string {
	return filepath.Join(s.cfg.Transfer.InboxDir, safeSegment(transferID))
}

func (s *Server) taskPath(transferID string) string {
	return filepath.Join(s.transferDir(transferID), "task.json")
}

func (s *Server) chunkPath(transferID string, chunkIndex int) string {
	return filepath.Join(s.transferDir(transferID), "chunks", fmt.Sprintf("%d.part", chunkIndex))
}

func parseP2PTransferPath(pathValue string) (string, string, bool) {
	trimmed := strings.Trim(strings.TrimPrefix(pathValue, "/p2p/v1/transfers/"), "/")
	if trimmed == pathValue || trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], strings.Join(parts[1:], "/"), true
}

func bearerToken(header string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("invalid bearer token")
	}
	return strings.TrimSpace(parts[1]), nil
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func appendUnique(existing []int, value int) []int {
	for _, current := range existing {
		if current == value {
			return existing
		}
	}
	return append(existing, value)
}

func safeFileName(value string) string {
	name := filepath.Base(strings.TrimSpace(value))
	if name == "." || name == ".." || name == string(filepath.Separator) || name == "" {
		return fmt.Sprintf("linknest-transfer-%d", time.Now().UnixNano())
	}
	return name
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, string(filepath.Separator), "_")
	if value == "" || value == "." || value == ".." {
		return fmt.Sprintf("transfer-%d", time.Now().UnixNano())
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
