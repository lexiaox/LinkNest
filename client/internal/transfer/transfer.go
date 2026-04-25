package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	clientconfig "linknest/client/internal/config"
	"linknest/client/internal/httpx"
)

type RemoteFile struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	FileHash string `json:"file_hash"`
	Status   string `json:"status"`
}

type RemoteTask struct {
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	FileName       string `json:"file_name"`
	TotalChunks    int    `json:"total_chunks"`
	UploadedChunks int    `json:"uploaded_chunks"`
	Status         string `json:"status"`
}

type LocalTask struct {
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	LocalPath      string `json:"local_path"`
	FileName       string `json:"file_name"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	Status         string `json:"status"`
}

type InitUploadRequest struct {
	DeviceID    string `json:"device_id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	FileHash    string `json:"file_hash"`
	ChunkSize   int64  `json:"chunk_size"`
	TotalChunks int    `json:"total_chunks"`
}

type InitUploadResponse struct {
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	ChunkSize      int64  `json:"chunk_size"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	MissingChunks  []int  `json:"missing_chunks"`
	Status         string `json:"status"`
}

type MissingChunksResponse struct {
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	TotalChunks    int    `json:"total_chunks"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	MissingChunks  []int  `json:"missing_chunks"`
	Status         string `json:"status"`
}

type CompleteUploadResponse struct {
	UploadID      string `json:"upload_id"`
	FileID        string `json:"file_id"`
	Status        string `json:"status"`
	MissingChunks []int  `json:"missing_chunks"`
}

type fileListResponse struct {
	Items []RemoteFile `json:"items"`
}

type taskListResponse struct {
	Items []RemoteTask `json:"items"`
}

type deleteFileResponse struct {
	FileID  string `json:"file_id"`
	Deleted bool   `json:"deleted"`
	Status  string `json:"status"`
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func ListFiles(baseURL string, token string) ([]RemoteFile, error) {
	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/files", nil)
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

	var result fileListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func ListTasks(baseURL string, token string) ([]RemoteTask, error) {
	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tasks", nil)
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

	var result taskListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func Upload(root string, cfg clientconfig.ClientConfig, localPath string) error {
	_, err := UploadWithResult(root, cfg, localPath)
	return err
}

func UploadWithResult(root string, cfg clientconfig.ClientConfig, localPath string) (CompleteUploadResponse, error) {
	if strings.TrimSpace(cfg.Device.DeviceID) == "" {
		return CompleteUploadResponse{}, errors.New("device is not initialized, run device init first")
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return CompleteUploadResponse{}, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return CompleteUploadResponse{}, err
	}
	if info.IsDir() {
		return CompleteUploadResponse{}, errors.New("file upload does not support directories in V1")
	}

	fileHash, err := ComputeSHA256(absPath)
	if err != nil {
		return CompleteUploadResponse{}, err
	}

	chunkSize := cfg.Transfer.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 4194304
	}
	totalChunks := int((info.Size() + chunkSize - 1) / chunkSize)

	initResp, err := initUpload(cfg.ServerURL, cfg.Token, InitUploadRequest{
		DeviceID:    cfg.Device.DeviceID,
		FileName:    filepath.Base(absPath),
		FileSize:    info.Size(),
		FileHash:    fileHash,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
	})
	if err != nil {
		return CompleteUploadResponse{}, err
	}

	if initResp.Status == "available" {
		fmt.Printf("file already available file_id=%s\n", initResp.FileID)
		return CompleteUploadResponse{UploadID: initResp.UploadID, FileID: initResp.FileID, Status: initResp.Status}, nil
	}

	task := LocalTask{
		UploadID:       initResp.UploadID,
		FileID:         initResp.FileID,
		LocalPath:      absPath,
		FileName:       filepath.Base(absPath),
		FileHash:       fileHash,
		ChunkSize:      chunkSize,
		TotalChunks:    totalChunks,
		UploadedChunks: initResp.UploadedChunks,
		Status:         initResp.Status,
	}
	if err := SaveTask(root, task); err != nil {
		return CompleteUploadResponse{}, err
	}

	if err := uploadMissingChunks(root, cfg, &task, initResp.MissingChunks); err != nil {
		return CompleteUploadResponse{}, err
	}

	result, err := completeUpload(cfg.ServerURL, cfg.Token, task.UploadID)
	if err != nil {
		return CompleteUploadResponse{}, err
	}
	if len(result.MissingChunks) > 0 {
		task.Status = "uploading"
		_ = SaveTask(root, task)
		return CompleteUploadResponse{}, fmt.Errorf("server reports missing chunks: %v", result.MissingChunks)
	}

	task.Status = result.Status
	if err := SaveTask(root, task); err != nil {
		return CompleteUploadResponse{}, err
	}

	fmt.Printf("upload completed upload_id=%s file_id=%s\n", result.UploadID, result.FileID)
	return result, nil
}

func Download(root string, cfg clientconfig.ClientConfig, fileID string, output string) error {
	_ = root

	files, err := ListFiles(cfg.ServerURL, cfg.Token)
	if err != nil {
		return err
	}

	var target RemoteFile
	found := false
	for _, item := range files {
		if item.FileID == fileID {
			target = item
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("file %s not found in file list", fileID)
	}

	destination := output
	if strings.TrimSpace(destination) == "" {
		destination = target.FileName
	}
	absOutput, err := filepath.Abs(destination)
	if err != nil {
		return err
	}

	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimRight(cfg.ServerURL, "/")+"/api/files/"+fileID+"/download", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, readErr := ioutil.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		return decodeError(body, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(absOutput), 0755); err != nil {
		return err
	}

	outputFile, err := os.Create(absOutput)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(outputFile, hasher), resp.Body); err != nil {
		outputFile.Close()
		os.Remove(absOutput)
		return err
	}
	if err := outputFile.Close(); err != nil {
		os.Remove(absOutput)
		return err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	expectedHash := strings.ToLower(strings.TrimSpace(target.FileHash))
	if headerHash := strings.TrimSpace(resp.Header.Get("X-File-Hash")); headerHash != "" {
		expectedHash = strings.ToLower(headerHash)
	}

	if expectedHash != "" && actualHash != expectedHash {
		os.Remove(absOutput)
		return fmt.Errorf("download hash mismatch: got %s want %s", actualHash, expectedHash)
	}

	fmt.Printf("download completed file_id=%s output=%s\n", fileID, absOutput)
	return nil
}

func DeleteFile(baseURL string, token string, fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return errors.New("file_id is required")
	}

	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodDelete, strings.TrimRight(baseURL, "/")+"/api/files/"+fileID, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
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
		return decodeError(body, resp.Status)
	}

	var result deleteFileResponse
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			return err
		}
	}

	fmt.Printf("file deleted file_id=%s status=%s\n", fileID, strings.TrimSpace(result.Status))
	return nil
}

func Resume(root string, cfg clientconfig.ClientConfig, uploadID string) error {
	task, err := LoadTask(root, uploadID)
	if err != nil {
		return err
	}

	progress, err := missingChunks(cfg.ServerURL, cfg.Token, uploadID)
	if err != nil {
		return err
	}

	task.UploadedChunks = progress.UploadedChunks
	task.Status = progress.Status
	task.TotalChunks = progress.TotalChunks
	if err := SaveTask(root, task); err != nil {
		return err
	}

	if len(progress.MissingChunks) > 0 {
		if err := uploadMissingChunks(root, cfg, &task, progress.MissingChunks); err != nil {
			return err
		}
	}

	result, err := completeUpload(cfg.ServerURL, cfg.Token, uploadID)
	if err != nil {
		return err
	}
	if len(result.MissingChunks) > 0 {
		task.Status = "uploading"
		task.UploadedChunks = complementIndexes(task.TotalChunks, result.MissingChunks)
		_ = SaveTask(root, task)
		return fmt.Errorf("server reports missing chunks: %v", result.MissingChunks)
	}

	task.Status = result.Status
	if err := SaveTask(root, task); err != nil {
		return err
	}

	fmt.Printf("resume completed upload_id=%s file_id=%s\n", result.UploadID, result.FileID)
	return nil
}

func SaveTask(root string, task LocalTask) error {
	raw, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(taskPath(root, task.UploadID), raw, 0644)
}

func LoadTask(root string, uploadID string) (LocalTask, error) {
	raw, err := ioutil.ReadFile(taskPath(root, uploadID))
	if err != nil {
		return LocalTask{}, err
	}

	var task LocalTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return LocalTask{}, err
	}
	return task, nil
}

func ComputeSHA256(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadMissingChunks(root string, cfg clientconfig.ClientConfig, task *LocalTask, missing []int) error {
	sort.Ints(missing)

	for _, chunkIndex := range missing {
		chunk, err := readChunk(task.LocalPath, int64(chunkIndex)*task.ChunkSize, task.ChunkSize)
		if err != nil {
			return err
		}
		if len(chunk) == 0 {
			return fmt.Errorf("chunk %d is empty", chunkIndex)
		}
		chunkHash := hashBytes(chunk)
		if err := uploadChunk(cfg.ServerURL, cfg.Token, task.UploadID, chunkIndex, chunkHash, chunk); err != nil {
			return err
		}
		task.UploadedChunks = appendUniqueChunk(task.UploadedChunks, chunkIndex)
		task.Status = "uploading"
		if err := SaveTask(root, *task); err != nil {
			return err
		}
	}
	return nil
}

func initUpload(baseURL string, token string, payload InitUploadRequest) (InitUploadResponse, error) {
	var result InitUploadResponse
	if err := doJSON(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/files/init-upload", token, payload, &result); err != nil {
		return InitUploadResponse{}, err
	}
	return result, nil
}

func missingChunks(baseURL string, token string, uploadID string) (MissingChunksResponse, error) {
	var result MissingChunksResponse
	if err := doJSON(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/uploads/"+uploadID+"/missing-chunks", token, nil, &result); err != nil {
		return MissingChunksResponse{}, err
	}
	return result, nil
}

func completeUpload(baseURL string, token string, uploadID string) (CompleteUploadResponse, error) {
	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/uploads/"+uploadID+"/complete", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return CompleteUploadResponse{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return CompleteUploadResponse{}, err
	}

	var result CompleteUploadResponse
	if err := json.Unmarshal(body, &result); err == nil && result.UploadID != "" {
		if resp.StatusCode >= 400 && len(result.MissingChunks) == 0 {
			return CompleteUploadResponse{}, decodeError(body, resp.Status)
		}
		return result, nil
	}
	if resp.StatusCode >= 400 {
		return CompleteUploadResponse{}, decodeError(body, resp.Status)
	}
	return CompleteUploadResponse{}, fmt.Errorf("unexpected complete upload response")
}

func uploadChunk(baseURL string, token string, uploadID string, chunkIndex int, chunkHash string, body []byte) error {
	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPut, strings.TrimRight(baseURL, "/")+"/api/uploads/"+uploadID+"/chunks/"+strconv.Itoa(chunkIndex), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Chunk-Hash", chunkHash)
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return decodeError(bodyBytes, resp.Status)
	}
	return nil
}

func doJSON(method string, url string, token string, payload interface{}, out interface{}) error {
	var raw []byte
	if payload != nil {
		var err error
		raw, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	resp, err := httpx.Do(httpx.NewClient(30*time.Second), 2, func() (*http.Request, error) {
		var bodyReader io.Reader
		if raw != nil {
			bodyReader = bytes.NewReader(raw)
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
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
		return decodeError(body, resp.Status)
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func decodeError(body []byte, status string) error {
	var remoteErr errorBody
	if err := json.Unmarshal(body, &remoteErr); err == nil && remoteErr.Error.Message != "" {
		return fmt.Errorf("%s: %s", remoteErr.Error.Code, remoteErr.Error.Message)
	}
	return fmt.Errorf("request failed: %s", status)
}

func readChunk(path string, offset int64, size int64) ([]byte, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer input.Close()

	if _, err := input.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return ioutil.ReadAll(io.LimitReader(input, size))
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func taskPath(root string, uploadID string) string {
	return filepath.Join(root, "tasks", uploadID+".json")
}

func appendUniqueChunk(existing []int, chunkIndex int) []int {
	for _, current := range existing {
		if current == chunkIndex {
			sort.Ints(existing)
			return existing
		}
	}
	existing = append(existing, chunkIndex)
	sort.Ints(existing)
	return existing
}

func complementIndexes(total int, missing []int) []int {
	missingSet := make(map[int]struct{}, len(missing))
	for _, item := range missing {
		missingSet[item] = struct{}{}
	}

	var uploaded []int
	for index := 0; index < total; index++ {
		if _, ok := missingSet[index]; ok {
			continue
		}
		uploaded = append(uploaded, index)
	}
	return uploaded
}
