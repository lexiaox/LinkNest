package transfer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"linknest/server/internal/device"
	"linknest/server/internal/p2ptoken"

	"github.com/google/uuid"
)

var (
	ErrTransferNotFound = errors.New("transfer not found")
	ErrDeviceNotFound   = errors.New("device not found")
	ErrInvalidToken     = errors.New("invalid transfer token")
)

const (
	RouteP2P   = "p2p"
	RouteCloud = "cloud"

	StatusInitialized       = "initialized"
	StatusProbing           = "probing"
	StatusTransferring      = "transferring"
	StatusFallbackUploading = "fallback_uploading"
	StatusCompleted         = "completed"
	StatusFailed            = "failed"
)

type Service struct {
	db     *sql.DB
	device *device.Service
	tokens *p2ptoken.Service
}

type Candidate struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	NetworkType string `json:"network_type"`
	RTTMS       int    `json:"rtt_ms,omitempty"`
}

type TargetDevice struct {
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	Status      string `json:"status"`
	P2PEnabled  bool   `json:"p2p_enabled"`
	P2PPort     int    `json:"p2p_port,omitempty"`
	P2PProtocol string `json:"p2p_protocol,omitempty"`
}

type Task struct {
	ID                int64  `json:"id"`
	UserID            int64  `json:"user_id"`
	TransferID        string `json:"transfer_id"`
	SourceDeviceID    string `json:"source_device_id"`
	TargetDeviceID    string `json:"target_device_id"`
	FileID            string `json:"file_id,omitempty"`
	FileName          string `json:"file_name"`
	FileSize          int64  `json:"file_size"`
	FileHash          string `json:"file_hash"`
	ChunkSize         int64  `json:"chunk_size"`
	TotalChunks       int    `json:"total_chunks"`
	PreferredRoute    string `json:"preferred_route"`
	ActualRoute       string `json:"actual_route,omitempty"`
	Status            string `json:"status"`
	SelectedCandidate string `json:"selected_candidate,omitempty"`
	ErrorCode         string `json:"error_code,omitempty"`
	ErrorMessage      string `json:"error_message,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type InitInput struct {
	SourceDeviceID string `json:"source_device_id"`
	TargetDeviceID string `json:"target_device_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
}

type InitResult struct {
	TransferID     string       `json:"transfer_id"`
	PreferredRoute string       `json:"preferred_route"`
	FallbackRoute  string       `json:"fallback_route"`
	ExpiresAt      string       `json:"expires_at"`
	TransferToken  string       `json:"transfer_token,omitempty"`
	TargetDevice   TargetDevice `json:"target_device"`
	P2PCandidates  []Candidate  `json:"p2p_candidates"`
	Status         string       `json:"status"`
}

type ProbeResultInput struct {
	Success           bool      `json:"success"`
	SelectedCandidate Candidate `json:"selected_candidate"`
	ErrorCode         string    `json:"error_code"`
	ErrorMessage      string    `json:"error_message"`
}

type CompleteInput struct {
	Route          string `json:"route"`
	FileID         string `json:"file_id"`
	FileHash       string `json:"file_hash"`
	ReceivedChunks int    `json:"received_chunks"`
}

type FallbackInput struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type FallbackResult struct {
	Route         string `json:"route"`
	NextAction    string `json:"next_action"`
	InitUploadURL string `json:"init_upload_url"`
}

type ValidateTokenInput struct {
	TransferToken string `json:"transfer_token"`
	TransferID    string `json:"transfer_id"`
	DeviceID      string `json:"device_id"`
}

type ValidateTokenResult struct {
	TransferID     string `json:"transfer_id"`
	SourceDeviceID string `json:"source_device_id"`
	TargetDeviceID string `json:"target_device_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	Valid          bool   `json:"valid"`
}

func NewService(db *sql.DB, deviceService *device.Service, tokenService *p2ptoken.Service) *Service {
	return &Service{db: db, device: deviceService, tokens: tokenService}
}

func (s *Service) Init(ctx context.Context, userID int64, input InitInput) (InitResult, error) {
	input.SourceDeviceID = strings.TrimSpace(input.SourceDeviceID)
	input.TargetDeviceID = strings.TrimSpace(input.TargetDeviceID)
	input.FileName = strings.TrimSpace(input.FileName)
	input.FileHash = strings.TrimSpace(input.FileHash)

	if input.SourceDeviceID == "" || input.TargetDeviceID == "" || input.FileName == "" || input.FileHash == "" {
		return InitResult{}, errors.New("source_device_id, target_device_id, file_name and file_hash are required")
	}
	if input.FileSize <= 0 || input.ChunkSize <= 0 || input.TotalChunks <= 0 {
		return InitResult{}, errors.New("file_size, chunk_size and total_chunks must be greater than 0")
	}

	if _, err := s.device.GetByDeviceID(ctx, userID, input.SourceDeviceID); err != nil {
		return InitResult{}, ErrDeviceNotFound
	}
	target, err := s.device.GetByDeviceID(ctx, userID, input.TargetDeviceID)
	if err != nil {
		return InitResult{}, ErrDeviceNotFound
	}

	candidates := p2pCandidates(target)
	preferredRoute := RouteCloud
	status := StatusFallbackUploading
	if len(candidates) > 0 {
		preferredRoute = RouteP2P
		status = StatusInitialized
	}

	transferID := uuid.New().String()
	token, expiresAt, err := s.tokens.Issue(p2ptoken.IssueInput{
		TransferID:     transferID,
		UserID:         userID,
		SourceDeviceID: input.SourceDeviceID,
		TargetDeviceID: input.TargetDeviceID,
		FileName:       input.FileName,
		FileSize:       input.FileSize,
		FileHash:       input.FileHash,
		ChunkSize:      input.ChunkSize,
		TotalChunks:    input.TotalChunks,
	})
	if err != nil {
		return InitResult{}, err
	}

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO transfer_tasks (
	user_id, transfer_id, source_device_id, target_device_id, file_name, file_size, file_hash,
	chunk_size, total_chunks, preferred_route, status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, userID, transferID, input.SourceDeviceID, input.TargetDeviceID, input.FileName, input.FileSize, input.FileHash, input.ChunkSize, input.TotalChunks, preferredRoute, status); err != nil {
		return InitResult{}, fmt.Errorf("insert transfer task: %w", err)
	}

	return InitResult{
		TransferID:     transferID,
		PreferredRoute: preferredRoute,
		FallbackRoute:  RouteCloud,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
		TransferToken:  token,
		TargetDevice: TargetDevice{
			DeviceID:    target.DeviceID,
			DeviceName:  target.DeviceName,
			Status:      target.Status,
			P2PEnabled:  target.P2PEnabled,
			P2PPort:     target.P2PPort,
			P2PProtocol: target.P2PProtocol,
		},
		P2PCandidates: candidates,
		Status:        status,
	}, nil
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, transfer_id, source_device_id, target_device_id, COALESCE(file_id, ''), file_name,
       file_size, file_hash, chunk_size, total_chunks, preferred_route, COALESCE(actual_route, ''),
       status, COALESCE(selected_candidate, ''), COALESCE(error_code, ''), COALESCE(error_message, ''),
       created_at, updated_at
FROM transfer_tasks
WHERE user_id = ?
ORDER BY updated_at DESC, id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list transfers: %w", err)
	}
	defer rows.Close()

	var items []Task
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transfers: %w", err)
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, userID int64, transferID string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, transfer_id, source_device_id, target_device_id, COALESCE(file_id, ''), file_name,
       file_size, file_hash, chunk_size, total_chunks, preferred_route, COALESCE(actual_route, ''),
       status, COALESCE(selected_candidate, ''), COALESCE(error_code, ''), COALESCE(error_message, ''),
       created_at, updated_at
FROM transfer_tasks
WHERE user_id = ? AND transfer_id = ?
`, userID, strings.TrimSpace(transferID))
	return scanTask(row)
}

func (s *Service) ReportProbeResult(ctx context.Context, userID int64, transferID string, input ProbeResultInput) (Task, error) {
	task, err := s.Get(ctx, userID, transferID)
	if err != nil {
		return Task{}, err
	}

	if input.Success {
		raw, _ := json.Marshal(input.SelectedCandidate)
		if _, err := s.db.ExecContext(ctx, `
UPDATE transfer_tasks
SET status = ?, selected_candidate = ?, error_code = '', error_message = '', updated_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND transfer_id = ?
`, StatusTransferring, string(raw), userID, task.TransferID); err != nil {
			return Task{}, fmt.Errorf("update probe success: %w", err)
		}
		return s.Get(ctx, userID, transferID)
	}

	if _, err := s.db.ExecContext(ctx, `
UPDATE transfer_tasks
SET status = ?, actual_route = ?, error_code = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND transfer_id = ?
`, StatusFallbackUploading, RouteCloud, strings.TrimSpace(input.ErrorCode), strings.TrimSpace(input.ErrorMessage), userID, task.TransferID); err != nil {
		return Task{}, fmt.Errorf("update probe failure: %w", err)
	}
	return s.Get(ctx, userID, transferID)
}

func (s *Service) Complete(ctx context.Context, userID int64, transferID string, input CompleteInput) (Task, error) {
	task, err := s.Get(ctx, userID, transferID)
	if err != nil {
		return Task{}, err
	}

	route := strings.TrimSpace(input.Route)
	if route == "" {
		route = task.PreferredRoute
	}
	if route != RouteP2P && route != RouteCloud {
		return Task{}, errors.New("route must be p2p or cloud")
	}
	if strings.TrimSpace(input.FileHash) != "" && !strings.EqualFold(strings.TrimSpace(input.FileHash), task.FileHash) {
		return Task{}, errors.New("file hash mismatch")
	}

	if _, err := s.db.ExecContext(ctx, `
UPDATE transfer_tasks
SET status = ?, actual_route = ?, file_id = NULLIF(?, ''), error_code = '', error_message = '', updated_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND transfer_id = ?
`, StatusCompleted, route, strings.TrimSpace(input.FileID), userID, task.TransferID); err != nil {
		return Task{}, fmt.Errorf("complete transfer: %w", err)
	}
	return s.Get(ctx, userID, transferID)
}

func (s *Service) Fallback(ctx context.Context, userID int64, transferID string, input FallbackInput) (FallbackResult, error) {
	if _, err := s.Get(ctx, userID, transferID); err != nil {
		return FallbackResult{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE transfer_tasks
SET status = ?, actual_route = ?, error_code = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND transfer_id = ?
`, StatusFallbackUploading, RouteCloud, strings.TrimSpace(input.Reason), strings.TrimSpace(input.Message), userID, strings.TrimSpace(transferID)); err != nil {
		return FallbackResult{}, fmt.Errorf("fallback transfer: %w", err)
	}
	return FallbackResult{
		Route:         RouteCloud,
		NextAction:    "init_upload",
		InitUploadURL: "/api/files/init-upload",
	}, nil
}

func (s *Service) ValidateToken(ctx context.Context, userID int64, input ValidateTokenInput) (ValidateTokenResult, error) {
	claims, err := s.tokens.Parse(input.TransferToken)
	if err != nil {
		return ValidateTokenResult{}, ErrInvalidToken
	}
	if claims.UserID != userID ||
		claims.TransferID != strings.TrimSpace(input.TransferID) ||
		claims.TargetDeviceID != strings.TrimSpace(input.DeviceID) {
		return ValidateTokenResult{}, ErrInvalidToken
	}

	if _, err := s.device.GetByDeviceID(ctx, userID, claims.TargetDeviceID); err != nil {
		return ValidateTokenResult{}, ErrInvalidToken
	}
	return ValidateTokenResult{
		TransferID:     claims.TransferID,
		SourceDeviceID: claims.SourceDeviceID,
		TargetDeviceID: claims.TargetDeviceID,
		FileName:       claims.FileName,
		FileSize:       claims.FileSize,
		FileHash:       claims.FileHash,
		ChunkSize:      claims.ChunkSize,
		TotalChunks:    claims.TotalChunks,
		Valid:          true,
	}, nil
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTask(row scanner) (Task, error) {
	var item Task
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.TransferID,
		&item.SourceDeviceID,
		&item.TargetDeviceID,
		&item.FileID,
		&item.FileName,
		&item.FileSize,
		&item.FileHash,
		&item.ChunkSize,
		&item.TotalChunks,
		&item.PreferredRoute,
		&item.ActualRoute,
		&item.Status,
		&item.SelectedCandidate,
		&item.ErrorCode,
		&item.ErrorMessage,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return Task{}, ErrTransferNotFound
		}
		return Task{}, fmt.Errorf("scan transfer: %w", err)
	}
	return item, nil
}

func p2pCandidates(target device.Device) []Candidate {
	if target.Status != "online" || !target.P2PEnabled || target.P2PPort <= 0 {
		return nil
	}
	protocol := strings.TrimSpace(target.P2PProtocol)
	if protocol == "" {
		protocol = "http"
	}

	var candidates []Candidate
	if host := strings.TrimSpace(target.LanIP); host != "" {
		candidates = append(candidates, Candidate{Host: host, Port: target.P2PPort, Protocol: protocol, NetworkType: "lan"})
	}
	if host := strings.TrimSpace(target.VirtualIP); host != "" {
		candidates = append(candidates, Candidate{Host: host, Port: target.P2PPort, Protocol: protocol, NetworkType: "virtual_lan"})
	}
	return candidates
}
