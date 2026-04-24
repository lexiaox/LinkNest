package file

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"linknest/server/internal/storage"

	"github.com/google/uuid"
)

var (
	ErrFileNotFound        = errors.New("file not found")
	ErrFileNotAvailable    = errors.New("file not available")
	ErrUploadNotFound      = errors.New("upload not found")
	ErrChunkOutOfRange     = errors.New("chunk out of range")
	ErrChunkHashMismatch   = errors.New("chunk hash mismatch")
	ErrChunkConflict       = errors.New("chunk conflict")
	ErrMissingChunks       = errors.New("missing chunks")
	ErrFileHashMismatch    = errors.New("file hash mismatch")
	ErrDeviceNotRegistered = errors.New("device not registered")
)

type Service struct {
	db      *sql.DB
	storage storage.Local
}

type Record struct {
	ID               int64  `json:"id"`
	UserID           int64  `json:"user_id"`
	FileID           string `json:"file_id"`
	FileName         string `json:"file_name"`
	FileSize         int64  `json:"file_size"`
	FileHash         string `json:"file_hash"`
	MIMEType         string `json:"mime_type,omitempty"`
	UploaderDeviceID string `json:"uploader_device_id,omitempty"`
	StoragePath      string `json:"storage_path,omitempty"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type InitUploadInput struct {
	DeviceID    string `json:"device_id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	FileHash    string `json:"file_hash"`
	ChunkSize   int64  `json:"chunk_size"`
	TotalChunks int    `json:"total_chunks"`
}

type InitUploadResult struct {
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	ChunkSize      int64  `json:"chunk_size"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	MissingChunks  []int  `json:"missing_chunks"`
	Status         string `json:"status"`
}

type MissingChunksResult struct {
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	TotalChunks    int    `json:"total_chunks"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	MissingChunks  []int  `json:"missing_chunks"`
	Status         string `json:"status"`
}

type ChunkUploadResult struct {
	UploadID   string `json:"upload_id"`
	ChunkIndex int    `json:"chunk_index"`
	Status     string `json:"status"`
}

type CompleteUploadResult struct {
	UploadID      string `json:"upload_id"`
	FileID        string `json:"file_id"`
	Status        string `json:"status"`
	MissingChunks []int  `json:"missing_chunks,omitempty"`
}

type uploadTaskRow struct {
	ID             int64
	UserID         int64
	UploadID       string
	FileID         string
	FileName       string
	FileSize       int64
	FileHash       string
	ChunkSize      int64
	TotalChunks    int
	UploadedChunks int
	Status         string
	ErrorMessage   string
}

type chunkRecord struct {
	ChunkIndex  int
	ChunkHash   string
	ChunkSize   int64
	StoragePath string
}

func NewService(db *sql.DB, localStorage storage.Local) *Service {
	return &Service{
		db:      db,
		storage: localStorage,
	}
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, file_id, file_name, file_size, file_hash, COALESCE(mime_type, ''), COALESCE(uploader_device_id, ''),
       COALESCE(storage_path, ''), status, created_at, updated_at
FROM files
WHERE user_id = ? AND status <> 'deleted'
ORDER BY created_at DESC, id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var items []Record
	for rows.Next() {
		var item Record
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.FileID,
			&item.FileName,
			&item.FileSize,
			&item.FileHash,
			&item.MIMEType,
			&item.UploaderDeviceID,
			&item.StoragePath,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate files: %w", err)
	}
	return items, nil
}

func (s *Service) InitUpload(ctx context.Context, userID int64, input InitUploadInput) (InitUploadResult, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.FileName = strings.TrimSpace(input.FileName)
	input.FileHash = strings.ToLower(strings.TrimSpace(input.FileHash))

	if input.DeviceID == "" || input.FileName == "" || input.FileHash == "" {
		return InitUploadResult{}, errors.New("device_id, file_name and file_hash are required")
	}
	if input.FileSize < 0 || input.ChunkSize <= 0 || input.TotalChunks <= 0 {
		return InitUploadResult{}, errors.New("file_size, chunk_size and total_chunks must be greater than 0")
	}
	if err := s.ensureDeviceBelongsToUser(ctx, userID, input.DeviceID); err != nil {
		return InitUploadResult{}, err
	}

	record, found, err := s.findAvailableFile(ctx, userID, input.FileHash, input.FileSize)
	if err != nil {
		return InitUploadResult{}, err
	}
	if found {
		return InitUploadResult{
			FileID:         record.FileID,
			ChunkSize:      input.ChunkSize,
			UploadedChunks: allChunkIndexes(input.TotalChunks),
			MissingChunks:  []int{},
			Status:         "available",
		}, nil
	}

	task, found, err := s.findReusableTask(ctx, userID, input.FileHash, input.FileSize)
	if err != nil {
		return InitUploadResult{}, err
	}
	if found {
		progress, err := s.GetMissingChunks(ctx, userID, task.UploadID)
		if err != nil {
			return InitUploadResult{}, err
		}
		return InitUploadResult{
			UploadID:       task.UploadID,
			FileID:         task.FileID,
			ChunkSize:      task.ChunkSize,
			UploadedChunks: progress.UploadedChunks,
			MissingChunks:  progress.MissingChunks,
			Status:         task.Status,
		}, nil
	}

	fileID := uuid.New().String()
	uploadID := uuid.New().String()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InitUploadResult{}, fmt.Errorf("begin init upload tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO files (user_id, file_id, file_name, file_size, file_hash, uploader_device_id, status)
VALUES (?, ?, ?, ?, ?, ?, 'uploading')
`, userID, fileID, input.FileName, input.FileSize, input.FileHash, input.DeviceID); err != nil {
		return InitUploadResult{}, fmt.Errorf("insert file: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO upload_tasks (user_id, upload_id, file_id, file_name, file_size, file_hash, chunk_size, total_chunks, uploaded_chunks, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 'initialized')
`, userID, uploadID, fileID, input.FileName, input.FileSize, input.FileHash, input.ChunkSize, input.TotalChunks); err != nil {
		return InitUploadResult{}, fmt.Errorf("insert upload task: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return InitUploadResult{}, fmt.Errorf("commit init upload tx: %w", err)
	}

	return InitUploadResult{
		UploadID:       uploadID,
		FileID:         fileID,
		ChunkSize:      input.ChunkSize,
		UploadedChunks: []int{},
		MissingChunks:  allChunkIndexes(input.TotalChunks),
		Status:         "initialized",
	}, nil
}

func (s *Service) GetMissingChunks(ctx context.Context, userID int64, uploadID string) (MissingChunksResult, error) {
	task, err := s.loadUploadTask(ctx, userID, uploadID)
	if err != nil {
		return MissingChunksResult{}, err
	}

	uploaded, err := s.listUploadedChunkIndexes(ctx, uploadID)
	if err != nil {
		return MissingChunksResult{}, err
	}

	return MissingChunksResult{
		UploadID:       task.UploadID,
		FileID:         task.FileID,
		TotalChunks:    task.TotalChunks,
		UploadedChunks: uploaded,
		MissingChunks:  diffChunkIndexes(task.TotalChunks, uploaded),
		Status:         task.Status,
	}, nil
}

func (s *Service) UploadChunk(ctx context.Context, userID int64, uploadID string, chunkIndex int, expectedHash string, body io.Reader) (ChunkUploadResult, error) {
	task, err := s.loadUploadTask(ctx, userID, uploadID)
	if err != nil {
		return ChunkUploadResult{}, err
	}
	if chunkIndex < 0 || chunkIndex >= task.TotalChunks {
		return ChunkUploadResult{}, ErrChunkOutOfRange
	}

	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))
	if expectedHash == "" {
		return ChunkUploadResult{}, ErrChunkHashMismatch
	}

	existing, found, err := s.findChunk(ctx, uploadID, chunkIndex)
	if err != nil {
		return ChunkUploadResult{}, err
	}
	if found {
		if existing.ChunkHash == expectedHash {
			return ChunkUploadResult{
				UploadID:   uploadID,
				ChunkIndex: chunkIndex,
				Status:     "uploaded",
			}, nil
		}
		return ChunkUploadResult{}, ErrChunkConflict
	}

	chunkPath := s.storage.ChunkPath(userID, uploadID, chunkIndex)
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0755); err != nil {
		return ChunkUploadResult{}, fmt.Errorf("create chunk dir: %w", err)
	}

	tempPath := chunkPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return ChunkUploadResult{}, fmt.Errorf("create chunk file: %w", err)
	}

	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hasher), body)
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(tempPath)
		return ChunkUploadResult{}, fmt.Errorf("write chunk: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return ChunkUploadResult{}, fmt.Errorf("close chunk file: %w", closeErr)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		os.Remove(tempPath)
		return ChunkUploadResult{}, ErrChunkHashMismatch
	}

	if size > task.ChunkSize && chunkIndex != task.TotalChunks-1 {
		os.Remove(tempPath)
		return ChunkUploadResult{}, ErrChunkOutOfRange
	}

	if err := os.Rename(tempPath, chunkPath); err != nil {
		os.Remove(tempPath)
		return ChunkUploadResult{}, fmt.Errorf("move chunk file: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ChunkUploadResult{}, fmt.Errorf("begin upload chunk tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO file_chunks (upload_id, file_id, chunk_index, chunk_hash, chunk_size, storage_path, status)
VALUES (?, ?, ?, ?, ?, ?, 'uploaded')
`, uploadID, task.FileID, chunkIndex, actualHash, size, chunkPath); err != nil {
		return ChunkUploadResult{}, fmt.Errorf("insert file chunk: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE upload_tasks
SET uploaded_chunks = (
	SELECT COUNT(*) FROM file_chunks WHERE upload_id = ?
),
    status = 'uploading',
    error_message = '',
    updated_at = CURRENT_TIMESTAMP
WHERE upload_id = ? AND user_id = ?
`, uploadID, uploadID, userID); err != nil {
		return ChunkUploadResult{}, fmt.Errorf("update upload task progress: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ChunkUploadResult{}, fmt.Errorf("commit upload chunk tx: %w", err)
	}

	return ChunkUploadResult{
		UploadID:   uploadID,
		ChunkIndex: chunkIndex,
		Status:     "uploaded",
	}, nil
}

func (s *Service) CompleteUpload(ctx context.Context, userID int64, uploadID string) (CompleteUploadResult, error) {
	task, err := s.loadUploadTask(ctx, userID, uploadID)
	if err != nil {
		return CompleteUploadResult{}, err
	}

	progress, err := s.GetMissingChunks(ctx, userID, uploadID)
	if err != nil {
		return CompleteUploadResult{}, err
	}
	if len(progress.MissingChunks) > 0 {
		return CompleteUploadResult{
			UploadID:      uploadID,
			FileID:        task.FileID,
			Status:        task.Status,
			MissingChunks: progress.MissingChunks,
		}, ErrMissingChunks
	}

	chunks, err := s.loadChunks(ctx, uploadID)
	if err != nil {
		return CompleteUploadResult{}, err
	}

	finalPath := s.storage.FinalPath(userID, task.FileID)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return CompleteUploadResult{}, fmt.Errorf("create final dir: %w", err)
	}

	tempPath := finalPath + ".tmp"
	output, err := os.Create(tempPath)
	if err != nil {
		return CompleteUploadResult{}, fmt.Errorf("create final file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(output, hasher)
	copyFailed := false

	for _, chunk := range chunks {
		input, err := os.Open(chunk.StoragePath)
		if err != nil {
			copyFailed = true
			output.Close()
			os.Remove(tempPath)
			return CompleteUploadResult{}, fmt.Errorf("open chunk %d: %w", chunk.ChunkIndex, err)
		}

		_, err = io.Copy(writer, input)
		input.Close()
		if err != nil {
			copyFailed = true
			output.Close()
			os.Remove(tempPath)
			return CompleteUploadResult{}, fmt.Errorf("merge chunk %d: %w", chunk.ChunkIndex, err)
		}
	}

	if err := output.Close(); err != nil {
		copyFailed = true
		os.Remove(tempPath)
		return CompleteUploadResult{}, fmt.Errorf("close final file: %w", err)
	}

	if copyFailed {
		os.Remove(tempPath)
		return CompleteUploadResult{}, errors.New("copy failed")
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != strings.ToLower(task.FileHash) {
		os.Remove(tempPath)
		if err := s.markTaskFailed(ctx, uploadID, userID, "merged file hash mismatch"); err != nil {
			return CompleteUploadResult{}, err
		}
		if _, err := s.db.ExecContext(ctx, `
UPDATE files SET status = 'failed', updated_at = CURRENT_TIMESTAMP WHERE file_id = ? AND user_id = ?
`, task.FileID, userID); err != nil {
			return CompleteUploadResult{}, fmt.Errorf("mark file failed: %w", err)
		}
		return CompleteUploadResult{}, ErrFileHashMismatch
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		os.Remove(tempPath)
		return CompleteUploadResult{}, fmt.Errorf("move final file: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompleteUploadResult{}, fmt.Errorf("begin complete upload tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE files
SET storage_path = ?, status = 'available', updated_at = CURRENT_TIMESTAMP
WHERE file_id = ? AND user_id = ?
`, finalPath, task.FileID, userID); err != nil {
		return CompleteUploadResult{}, fmt.Errorf("update file status: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE upload_tasks
SET uploaded_chunks = total_chunks, status = 'completed', error_message = '', updated_at = CURRENT_TIMESTAMP
WHERE upload_id = ? AND user_id = ?
`, uploadID, userID); err != nil {
		return CompleteUploadResult{}, fmt.Errorf("update upload task completed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CompleteUploadResult{}, fmt.Errorf("commit complete upload tx: %w", err)
	}

	os.RemoveAll(filepath.Dir(s.storage.ChunkPath(userID, uploadID, 0)))

	return CompleteUploadResult{
		UploadID: uploadID,
		FileID:   task.FileID,
		Status:   "completed",
	}, nil
}

func (s *Service) OpenDownload(ctx context.Context, userID int64, fileID string) (Record, error) {
	var item Record
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, file_id, file_name, file_size, file_hash, COALESCE(mime_type, ''), COALESCE(uploader_device_id, ''),
       COALESCE(storage_path, ''), status, created_at, updated_at
FROM files
WHERE user_id = ? AND file_id = ?
`, userID, fileID).Scan(
		&item.ID,
		&item.UserID,
		&item.FileID,
		&item.FileName,
		&item.FileSize,
		&item.FileHash,
		&item.MIMEType,
		&item.UploaderDeviceID,
		&item.StoragePath,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Record{}, ErrFileNotFound
		}
		return Record{}, fmt.Errorf("load download file: %w", err)
	}

	if item.Status == "deleted" {
		return Record{}, ErrFileNotFound
	}
	if item.Status != "available" {
		return Record{}, ErrFileNotAvailable
	}
	if strings.TrimSpace(item.StoragePath) == "" {
		return Record{}, ErrFileNotAvailable
	}
	if _, err := os.Stat(item.StoragePath); err != nil {
		if os.IsNotExist(err) {
			return Record{}, ErrFileNotAvailable
		}
		return Record{}, fmt.Errorf("stat download file: %w", err)
	}
	return item, nil
}

func (s *Service) Delete(ctx context.Context, userID int64, fileID string) error {
	record, err := s.loadFile(ctx, userID, fileID)
	if err != nil {
		return err
	}
	if record.Status == "deleted" {
		return ErrFileNotFound
	}

	uploadIDs, err := s.listUploadIDsByFile(ctx, userID, fileID)
	if err != nil {
		return err
	}
	chunkPaths, err := s.listChunkPathsByFile(ctx, fileID)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete file tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE files
SET status = 'deleted', updated_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND file_id = ?
`, userID, fileID); err != nil {
		return fmt.Errorf("mark file deleted: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM file_chunks
WHERE file_id = ?
`, fileID); err != nil {
		return fmt.Errorf("delete file chunks: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM upload_tasks
WHERE user_id = ? AND file_id = ?
`, userID, fileID); err != nil {
		return fmt.Errorf("delete upload tasks: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete file tx: %w", err)
	}

	if strings.TrimSpace(record.StoragePath) != "" {
		if err := os.Remove(record.StoragePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove final file: %w", err)
		}
	}

	for _, chunkPath := range chunkPaths {
		if strings.TrimSpace(chunkPath) == "" {
			continue
		}
		if err := os.Remove(chunkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove chunk file: %w", err)
		}
	}
	for _, uploadID := range uploadIDs {
		if strings.TrimSpace(uploadID) == "" {
			continue
		}
		if err := os.RemoveAll(filepath.Dir(s.storage.ChunkPath(userID, uploadID, 0))); err != nil {
			return fmt.Errorf("remove chunk dir: %w", err)
		}
	}

	return nil
}

func (s *Service) ensureDeviceBelongsToUser(ctx context.Context, userID int64, deviceID string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, `
SELECT 1
FROM devices
WHERE user_id = ? AND device_id = ?
`, userID, deviceID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrDeviceNotRegistered
		}
		return fmt.Errorf("check device exists: %w", err)
	}
	return nil
}

func (s *Service) findAvailableFile(ctx context.Context, userID int64, fileHash string, fileSize int64) (Record, bool, error) {
	var item Record
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, file_id, file_name, file_size, file_hash, COALESCE(mime_type, ''), COALESCE(uploader_device_id, ''),
       COALESCE(storage_path, ''), status, created_at, updated_at
FROM files
WHERE user_id = ? AND file_hash = ? AND file_size = ? AND status = 'available'
ORDER BY id DESC
LIMIT 1
`, userID, fileHash, fileSize).Scan(
		&item.ID,
		&item.UserID,
		&item.FileID,
		&item.FileName,
		&item.FileSize,
		&item.FileHash,
		&item.MIMEType,
		&item.UploaderDeviceID,
		&item.StoragePath,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Record{}, false, nil
		}
		return Record{}, false, fmt.Errorf("find available file: %w", err)
	}
	return item, true, nil
}

func (s *Service) loadFile(ctx context.Context, userID int64, fileID string) (Record, error) {
	var item Record
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, file_id, file_name, file_size, file_hash, COALESCE(mime_type, ''), COALESCE(uploader_device_id, ''),
       COALESCE(storage_path, ''), status, created_at, updated_at
FROM files
WHERE user_id = ? AND file_id = ?
`, userID, fileID).Scan(
		&item.ID,
		&item.UserID,
		&item.FileID,
		&item.FileName,
		&item.FileSize,
		&item.FileHash,
		&item.MIMEType,
		&item.UploaderDeviceID,
		&item.StoragePath,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Record{}, ErrFileNotFound
		}
		return Record{}, fmt.Errorf("load file: %w", err)
	}
	return item, nil
}

func (s *Service) findReusableTask(ctx context.Context, userID int64, fileHash string, fileSize int64) (uploadTaskRow, bool, error) {
	var item uploadTaskRow
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, upload_id, file_id, file_name, file_size, file_hash, chunk_size, total_chunks, uploaded_chunks,
       status, COALESCE(error_message, '')
FROM upload_tasks
WHERE user_id = ? AND file_hash = ? AND file_size = ? AND status IN ('initialized', 'uploading', 'failed')
ORDER BY id DESC
LIMIT 1
`, userID, fileHash, fileSize).Scan(
		&item.ID,
		&item.UserID,
		&item.UploadID,
		&item.FileID,
		&item.FileName,
		&item.FileSize,
		&item.FileHash,
		&item.ChunkSize,
		&item.TotalChunks,
		&item.UploadedChunks,
		&item.Status,
		&item.ErrorMessage,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return uploadTaskRow{}, false, nil
		}
		return uploadTaskRow{}, false, fmt.Errorf("find reusable task: %w", err)
	}
	return item, true, nil
}

func (s *Service) loadUploadTask(ctx context.Context, userID int64, uploadID string) (uploadTaskRow, error) {
	var item uploadTaskRow
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, upload_id, file_id, file_name, file_size, file_hash, chunk_size, total_chunks, uploaded_chunks,
       status, COALESCE(error_message, '')
FROM upload_tasks
WHERE user_id = ? AND upload_id = ?
`, userID, uploadID).Scan(
		&item.ID,
		&item.UserID,
		&item.UploadID,
		&item.FileID,
		&item.FileName,
		&item.FileSize,
		&item.FileHash,
		&item.ChunkSize,
		&item.TotalChunks,
		&item.UploadedChunks,
		&item.Status,
		&item.ErrorMessage,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return uploadTaskRow{}, ErrUploadNotFound
		}
		return uploadTaskRow{}, fmt.Errorf("load upload task: %w", err)
	}
	return item, nil
}

func (s *Service) listUploadIDsByFile(ctx context.Context, userID int64, fileID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT upload_id
FROM upload_tasks
WHERE user_id = ? AND file_id = ?
`, userID, fileID)
	if err != nil {
		return nil, fmt.Errorf("list upload ids: %w", err)
	}
	defer rows.Close()

	var uploadIDs []string
	for rows.Next() {
		var uploadID string
		if err := rows.Scan(&uploadID); err != nil {
			return nil, fmt.Errorf("scan upload id: %w", err)
		}
		uploadIDs = append(uploadIDs, uploadID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upload ids: %w", err)
	}
	return uploadIDs, nil
}

func (s *Service) listChunkPathsByFile(ctx context.Context, fileID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT COALESCE(storage_path, '')
FROM file_chunks
WHERE file_id = ?
`, fileID)
	if err != nil {
		return nil, fmt.Errorf("list chunk paths: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var current string
		if err := rows.Scan(&current); err != nil {
			return nil, fmt.Errorf("scan chunk path: %w", err)
		}
		paths = append(paths, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunk paths: %w", err)
	}
	return paths, nil
}

func (s *Service) listUploadedChunkIndexes(ctx context.Context, uploadID string) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT chunk_index
FROM file_chunks
WHERE upload_id = ?
ORDER BY chunk_index ASC
`, uploadID)
	if err != nil {
		return nil, fmt.Errorf("list uploaded chunks: %w", err)
	}
	defer rows.Close()

	var indexes []int
	for rows.Next() {
		var index int
		if err := rows.Scan(&index); err != nil {
			return nil, fmt.Errorf("scan uploaded chunk index: %w", err)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate uploaded chunks: %w", err)
	}
	return indexes, nil
}

func (s *Service) loadChunks(ctx context.Context, uploadID string) ([]chunkRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT chunk_index, chunk_hash, chunk_size, storage_path
FROM file_chunks
WHERE upload_id = ?
ORDER BY chunk_index ASC
`, uploadID)
	if err != nil {
		return nil, fmt.Errorf("load chunks: %w", err)
	}
	defer rows.Close()

	var chunks []chunkRecord
	for rows.Next() {
		var item chunkRecord
		if err := rows.Scan(&item.ChunkIndex, &item.ChunkHash, &item.ChunkSize, &item.StoragePath); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunks: %w", err)
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].ChunkIndex < chunks[j].ChunkIndex
	})
	return chunks, nil
}

func (s *Service) findChunk(ctx context.Context, uploadID string, chunkIndex int) (chunkRecord, bool, error) {
	var item chunkRecord
	err := s.db.QueryRowContext(ctx, `
SELECT chunk_index, chunk_hash, chunk_size, storage_path
FROM file_chunks
WHERE upload_id = ? AND chunk_index = ?
`, uploadID, chunkIndex).Scan(&item.ChunkIndex, &item.ChunkHash, &item.ChunkSize, &item.StoragePath)
	if err != nil {
		if err == sql.ErrNoRows {
			return chunkRecord{}, false, nil
		}
		return chunkRecord{}, false, fmt.Errorf("find chunk: %w", err)
	}
	return item, true, nil
}

func (s *Service) markTaskFailed(ctx context.Context, uploadID string, userID int64, message string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE upload_tasks
SET status = 'failed', error_message = ?, updated_at = CURRENT_TIMESTAMP
WHERE upload_id = ? AND user_id = ?
`, message, uploadID, userID)
	if err != nil {
		return fmt.Errorf("mark task failed: %w", err)
	}
	return nil
}

func allChunkIndexes(total int) []int {
	indexes := make([]int, 0, total)
	for i := 0; i < total; i++ {
		indexes = append(indexes, i)
	}
	return indexes
}

func diffChunkIndexes(total int, uploaded []int) []int {
	existing := make(map[int]struct{}, len(uploaded))
	for _, index := range uploaded {
		existing[index] = struct{}{}
	}

	missing := make([]int, 0, total-len(uploaded))
	for i := 0; i < total; i++ {
		if _, ok := existing[i]; !ok {
			missing = append(missing, i)
		}
	}
	return missing
}

func ComputeSHA256FromFile(path string) (string, error) {
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

func ReadChunk(path string, offset int64, size int64) ([]byte, error) {
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
