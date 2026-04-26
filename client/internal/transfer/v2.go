package transfer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/httpx"
)

type Candidate struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	NetworkType string `json:"network_type"`
	RTTMS       int    `json:"rtt_ms,omitempty"`
}

const (
	RouteP2P   = "p2p"
	RouteCloud = "cloud"
)

type TransferTask struct {
	TransferID        string `json:"transfer_id"`
	SourceDeviceID    string `json:"source_device_id"`
	TargetDeviceID    string `json:"target_device_id"`
	FileID            string `json:"file_id"`
	FileName          string `json:"file_name"`
	FileSize          int64  `json:"file_size"`
	FileHash          string `json:"file_hash"`
	ChunkSize         int64  `json:"chunk_size"`
	TotalChunks       int    `json:"total_chunks"`
	PreferredRoute    string `json:"preferred_route"`
	ActualRoute       string `json:"actual_route"`
	Status            string `json:"status"`
	SelectedCandidate string `json:"selected_candidate"`
	ErrorCode         string `json:"error_code"`
	ErrorMessage      string `json:"error_message"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type LocalTransfer struct {
	TransferID     string      `json:"transfer_id"`
	SourceDeviceID string      `json:"source_device_id"`
	TargetDeviceID string      `json:"target_device_id"`
	LocalPath      string      `json:"local_path"`
	FileName       string      `json:"file_name"`
	FileHash       string      `json:"file_hash"`
	ChunkSize      int64       `json:"chunk_size"`
	TotalChunks    int         `json:"total_chunks"`
	PreferredRoute string      `json:"preferred_route"`
	TransferToken  string      `json:"transfer_token,omitempty"`
	P2PCandidates  []Candidate `json:"p2p_candidates,omitempty"`
	Status         string      `json:"status"`
	UpdatedAt      string      `json:"updated_at"`
}

type initTransferRequest struct {
	SourceDeviceID string `json:"source_device_id"`
	TargetDeviceID string `json:"target_device_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
}

type initTransferResponse struct {
	TransferID     string      `json:"transfer_id"`
	PreferredRoute string      `json:"preferred_route"`
	FallbackRoute  string      `json:"fallback_route"`
	TransferToken  string      `json:"transfer_token"`
	P2PCandidates  []Candidate `json:"p2p_candidates"`
	Status         string      `json:"status"`
}

type transferListResponse struct {
	Items []TransferTask `json:"items"`
}

type probeRequest struct {
	TransferID     string `json:"transfer_id"`
	SourceDeviceID string `json:"source_device_id"`
	FileHash       string `json:"file_hash"`
}

type probeResultRequest struct {
	Success           bool      `json:"success"`
	SelectedCandidate Candidate `json:"selected_candidate,omitempty"`
	ErrorCode         string    `json:"error_code,omitempty"`
	ErrorMessage      string    `json:"error_message,omitempty"`
}

type fallbackRequest struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type completeTransferRequest struct {
	Route          string `json:"route"`
	FileID         string `json:"file_id,omitempty"`
	FileHash       string `json:"file_hash"`
	ReceivedChunks int    `json:"received_chunks,omitempty"`
}

func Send(root string, cfg clientconfig.ClientConfig, localPath string, targetDeviceID string) error {
	if strings.TrimSpace(cfg.Device.DeviceID) == "" {
		return errors.New("device is not initialized, run device init first")
	}
	if strings.TrimSpace(targetDeviceID) == "" {
		return errors.New("target device id is required")
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("transfer send does not support directories")
	}
	fileHash, err := ComputeSHA256(absPath)
	if err != nil {
		return err
	}
	chunkSize := cfg.Transfer.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 4194304
	}
	totalChunks := int((info.Size() + chunkSize - 1) / chunkSize)

	initResp, err := initTransfer(cfg, initTransferRequest{
		SourceDeviceID: cfg.Device.DeviceID,
		TargetDeviceID: strings.TrimSpace(targetDeviceID),
		FileName:       filepath.Base(absPath),
		FileSize:       info.Size(),
		FileHash:       fileHash,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
	})
	if err != nil {
		return err
	}
	local := LocalTransfer{
		TransferID:     initResp.TransferID,
		SourceDeviceID: cfg.Device.DeviceID,
		TargetDeviceID: strings.TrimSpace(targetDeviceID),
		LocalPath:      absPath,
		FileName:       filepath.Base(absPath),
		FileHash:       fileHash,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
		PreferredRoute: initResp.PreferredRoute,
		TransferToken:  initResp.TransferToken,
		P2PCandidates:  initResp.P2PCandidates,
		Status:         initResp.Status,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}
	if err := SaveLocalTransfer(root, local); err != nil {
		return err
	}

	if initResp.PreferredRoute == "p2p" && len(initResp.P2PCandidates) > 0 {
		candidate, err := probeCandidates(cfg, initResp, fileHash)
		if err == nil {
			if err := reportProbeResult(cfg, initResp.TransferID, probeResultRequest{Success: true, SelectedCandidate: candidate}); err != nil {
				return err
			}
			if err := sendP2PChunks(cfg, initResp, absPath, candidate, chunkSize, totalChunks, fileHash); err == nil {
				if err := completeTransfer(cfg, initResp.TransferID, completeTransferRequest{
					Route:          "p2p",
					FileHash:       fileHash,
					ReceivedChunks: totalChunks,
				}); err != nil {
					return err
				}
				local.Status = "completed"
				_ = SaveLocalTransfer(root, local)
				fmt.Printf("transfer completed route=p2p transfer_id=%s\n", initResp.TransferID)
				return nil
			} else if !clientconfig.FallbackToCloudEnabled(cfg.Transfer) {
				_ = markTransferFailed(cfg, initResp.TransferID, "P2P_TRANSFER_FAILED", err.Error())
				return fmt.Errorf("p2p transfer failed and cloud fallback is disabled: %w", err)
			} else {
				_, _ = fallbackTransfer(cfg, initResp.TransferID, "P2P_TRANSFER_INTERRUPTED", err.Error())
			}
		} else if !clientconfig.FallbackToCloudEnabled(cfg.Transfer) {
			_ = reportProbeResult(cfg, initResp.TransferID, probeResultRequest{Success: false, ErrorCode: "P2P_CONNECT_TIMEOUT", ErrorMessage: err.Error()})
			return fmt.Errorf("p2p probe failed and cloud fallback is disabled: %w", err)
		} else {
			_ = reportProbeResult(cfg, initResp.TransferID, probeResultRequest{Success: false, ErrorCode: "P2P_CONNECT_TIMEOUT", ErrorMessage: err.Error()})
			_, _ = fallbackTransfer(cfg, initResp.TransferID, "P2P_CONNECT_TIMEOUT", err.Error())
		}
	}

	if !clientconfig.FallbackToCloudEnabled(cfg.Transfer) {
		return errors.New("FALLBACK_DISABLED: cloud fallback is disabled")
	}
	result, err := UploadWithResult(root, cfg, absPath)
	if err != nil {
		return err
	}
	if err := completeTransfer(cfg, initResp.TransferID, completeTransferRequest{
		Route:    "cloud",
		FileID:   result.FileID,
		FileHash: fileHash,
	}); err != nil {
		return err
	}
	local.Status = "completed"
	_ = SaveLocalTransfer(root, local)
	fmt.Printf("transfer completed route=cloud transfer_id=%s file_id=%s\n", initResp.TransferID, result.FileID)
	return nil
}

func ResumeTransfer(root string, cfg clientconfig.ClientConfig, transferID string) error {
	task, err := LoadLocalTransfer(root, transferID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(task.LocalPath) == "" || strings.TrimSpace(task.TargetDeviceID) == "" {
		return errors.New("local transfer cache is missing local_path or target_device_id")
	}

	fileHash := strings.TrimSpace(task.FileHash)
	if fileHash == "" {
		fileHash, err = ComputeSHA256(task.LocalPath)
		if err != nil {
			return err
		}
	}
	chunkSize := task.ChunkSize
	if chunkSize <= 0 {
		chunkSize = cfg.Transfer.ChunkSize
	}
	if chunkSize <= 0 {
		chunkSize = 4194304
	}
	totalChunks := task.TotalChunks
	if totalChunks <= 0 {
		info, err := os.Stat(task.LocalPath)
		if err != nil {
			return err
		}
		totalChunks = int((info.Size() + chunkSize - 1) / chunkSize)
	}

	initResp := initTransferResponse{
		TransferID:     task.TransferID,
		PreferredRoute: task.PreferredRoute,
		TransferToken:  task.TransferToken,
		P2PCandidates:  task.P2PCandidates,
		Status:         task.Status,
	}
	if initResp.PreferredRoute == "" {
		initResp.PreferredRoute = RouteCloud
	}

	if initResp.PreferredRoute == RouteP2P && initResp.TransferToken != "" && len(initResp.P2PCandidates) > 0 {
		candidate, err := probeCandidates(cfg, initResp, fileHash)
		if err == nil {
			if err := reportProbeResult(cfg, task.TransferID, probeResultRequest{Success: true, SelectedCandidate: candidate}); err != nil {
				return err
			}
			if err := sendP2PChunks(cfg, initResp, task.LocalPath, candidate, chunkSize, totalChunks, fileHash); err == nil {
				if err := completeTransfer(cfg, task.TransferID, completeTransferRequest{
					Route:          RouteP2P,
					FileHash:       fileHash,
					ReceivedChunks: totalChunks,
				}); err != nil {
					return err
				}
				task.Status = "completed"
				task.UpdatedAt = time.Now().Format(time.RFC3339)
				_ = SaveLocalTransfer(root, task)
				fmt.Printf("transfer resumed route=p2p transfer_id=%s\n", task.TransferID)
				return nil
			} else if !clientconfig.FallbackToCloudEnabled(cfg.Transfer) {
				_ = markTransferFailed(cfg, task.TransferID, "P2P_TRANSFER_FAILED", err.Error())
				return fmt.Errorf("p2p transfer failed and cloud fallback is disabled: %w", err)
			} else {
				_, _ = fallbackTransfer(cfg, task.TransferID, "P2P_TRANSFER_INTERRUPTED", err.Error())
			}
		} else if !clientconfig.FallbackToCloudEnabled(cfg.Transfer) {
			_ = reportProbeResult(cfg, task.TransferID, probeResultRequest{Success: false, ErrorCode: "P2P_CONNECT_TIMEOUT", ErrorMessage: err.Error()})
			return fmt.Errorf("p2p probe failed and cloud fallback is disabled: %w", err)
		} else {
			_ = reportProbeResult(cfg, task.TransferID, probeResultRequest{Success: false, ErrorCode: "P2P_CONNECT_TIMEOUT", ErrorMessage: err.Error()})
			_, _ = fallbackTransfer(cfg, task.TransferID, "P2P_CONNECT_TIMEOUT", err.Error())
		}
	}

	if !clientconfig.FallbackToCloudEnabled(cfg.Transfer) {
		return errors.New("FALLBACK_DISABLED: cloud fallback is disabled")
	}
	result, err := UploadWithResult(root, cfg, task.LocalPath)
	if err != nil {
		return err
	}
	if err := completeTransfer(cfg, task.TransferID, completeTransferRequest{
		Route:    RouteCloud,
		FileID:   result.FileID,
		FileHash: fileHash,
	}); err != nil {
		return err
	}
	task.Status = "completed"
	task.UpdatedAt = time.Now().Format(time.RFC3339)
	_ = SaveLocalTransfer(root, task)
	fmt.Printf("transfer resumed route=cloud transfer_id=%s file_id=%s\n", task.TransferID, result.FileID)
	return nil
}

func ListTransfers(baseURL string, token string) ([]TransferTask, error) {
	var result transferListResponse
	if err := doJSON(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/transfers", token, nil, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func TransferDetail(baseURL string, token string, transferID string) (TransferTask, error) {
	var result TransferTask
	if err := doJSON(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/transfers/"+strings.TrimSpace(transferID)+"/detail", token, nil, &result); err != nil {
		return TransferTask{}, err
	}
	return result, nil
}

func RequestFallback(baseURL string, token string, transferID string) error {
	_, err := fallbackTransfer(clientconfig.ClientConfig{ServerURL: baseURL, Token: token}, transferID, "USER_REQUESTED_FALLBACK", "fallback requested from client")
	return err
}

func SaveLocalTransfer(root string, task LocalTransfer) error {
	raw, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(localTransferPath(root, task.TransferID), raw, 0644)
}

func LoadLocalTransfer(root string, transferID string) (LocalTransfer, error) {
	raw, err := ioutil.ReadFile(localTransferPath(root, transferID))
	if err != nil {
		return LocalTransfer{}, err
	}
	var task LocalTransfer
	if err := json.Unmarshal(raw, &task); err != nil {
		return LocalTransfer{}, err
	}
	return task, nil
}

func initTransfer(cfg clientconfig.ClientConfig, payload initTransferRequest) (initTransferResponse, error) {
	var result initTransferResponse
	if err := doJSON(http.MethodPost, strings.TrimRight(cfg.ServerURL, "/")+"/api/transfers/init", cfg.Token, payload, &result); err != nil {
		return initTransferResponse{}, err
	}
	return result, nil
}

func probeCandidates(cfg clientconfig.ClientConfig, initResp initTransferResponse, fileHash string) (Candidate, error) {
	candidates := append([]Candidate(nil), initResp.P2PCandidates...)
	sort.SliceStable(candidates, func(i int, j int) bool {
		return routePriority(candidates[i]) < routePriority(candidates[j])
	})

	var lastErr error
	for _, candidate := range candidates {
		start := time.Now()
		if err := probeCandidate(cfg, initResp, candidate, fileHash); err != nil {
			lastErr = err
			continue
		}
		candidate.RTTMS = int(time.Since(start).Milliseconds())
		return candidate, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no p2p candidates")
	}
	return Candidate{}, lastErr
}

func probeCandidate(cfg clientconfig.ClientConfig, initResp initTransferResponse, candidate Candidate, fileHash string) error {
	payload, err := json.Marshal(probeRequest{
		TransferID:     initResp.TransferID,
		SourceDeviceID: cfg.Device.DeviceID,
		FileHash:       fileHash,
	})
	if err != nil {
		return err
	}
	return doP2PJSON(cfg, http.MethodPost, candidate, "/p2p/v1/probe", initResp.TransferToken, payload, nil)
}

func sendP2PChunks(cfg clientconfig.ClientConfig, initResp initTransferResponse, path string, candidate Candidate, chunkSize int64, totalChunks int, fileHash string) error {
	for index := 0; index < totalChunks; index++ {
		chunk, err := readChunk(path, int64(index)*chunkSize, chunkSize)
		if err != nil {
			return err
		}
		if len(chunk) == 0 {
			return fmt.Errorf("chunk %d is empty", index)
		}
		if err := putP2PChunk(cfg, initResp, candidate, index, chunk); err != nil {
			return err
		}
	}

	raw, err := json.Marshal(map[string]interface{}{
		"file_hash":    fileHash,
		"total_chunks": totalChunks,
	})
	if err != nil {
		return err
	}
	return doP2PJSON(cfg, http.MethodPost, candidate, "/p2p/v1/transfers/"+initResp.TransferID+"/complete", initResp.TransferToken, raw, nil)
}

func putP2PChunk(cfg clientconfig.ClientConfig, initResp initTransferResponse, candidate Candidate, index int, chunk []byte) error {
	u := candidateURL(candidate, "/p2p/v1/transfers/"+initResp.TransferID+"/chunks/"+strconv.Itoa(index))
	client := httpx.NewClient(time.Duration(cfg.Transfer.P2PChunkTimeoutSeconds) * time.Second)
	req, err := newP2PRequest(http.MethodPut, u, initResp.TransferToken, bytes.NewReader(chunk), hashBytes(chunk))
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("p2p chunk failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func doP2PJSON(cfg clientconfig.ClientConfig, method string, candidate Candidate, path string, token string, raw []byte, out interface{}) error {
	u := candidateURL(candidate, path)
	client := httpx.NewClient(time.Duration(cfg.Transfer.P2PConnectTimeoutSeconds) * time.Second)
	req, err := newP2PRequest(method, u, token, bytes.NewReader(raw), "")
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("p2p request failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func newP2PRequest(method string, url string, token string, body *bytes.Reader, chunkHash string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if chunkHash != "" {
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Chunk-Hash", chunkHash)
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func markTransferFailed(cfg clientconfig.ClientConfig, transferID string, errorCode string, errorMessage string) error {
	var result TransferTask
	return doJSON(http.MethodPost, strings.TrimRight(cfg.ServerURL, "/")+"/api/transfers/"+strings.TrimSpace(transferID)+"/mark-failed", cfg.Token, map[string]string{
		"error_code":    errorCode,
		"error_message": errorMessage,
	}, &result)
}

func reportProbeResult(cfg clientconfig.ClientConfig, transferID string, payload probeResultRequest) error {
	var result TransferTask
	return doJSON(http.MethodPost, strings.TrimRight(cfg.ServerURL, "/")+"/api/transfers/"+transferID+"/probe-result", cfg.Token, payload, &result)
}

func completeTransfer(cfg clientconfig.ClientConfig, transferID string, payload completeTransferRequest) error {
	var result TransferTask
	return doJSON(http.MethodPost, strings.TrimRight(cfg.ServerURL, "/")+"/api/transfers/"+transferID+"/complete", cfg.Token, payload, &result)
}

func fallbackTransfer(cfg clientconfig.ClientConfig, transferID string, reason string, message string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := doJSON(http.MethodPost, strings.TrimRight(cfg.ServerURL, "/")+"/api/transfers/"+transferID+"/fallback", cfg.Token, fallbackRequest{
		Reason:  reason,
		Message: message,
	}, &result)
	return result, err
}

func candidateURL(candidate Candidate, path string) string {
	protocol := strings.TrimSpace(candidate.Protocol)
	if protocol == "" {
		protocol = "http"
	}
	u := url.URL{
		Scheme: protocol,
		Host:   net.JoinHostPort(candidate.Host, strconv.Itoa(candidate.Port)),
		Path:   path,
	}
	return u.String()
}

func routePriority(candidate Candidate) int {
	switch candidate.NetworkType {
	case "lan":
		return 0
	case "virtual_lan":
		return 1
	default:
		return 2
	}
}

func localTransferPath(root string, transferID string) string {
	return filepath.Join(root, "transfers", strings.TrimSpace(transferID)+".json")
}
