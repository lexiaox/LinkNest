package file

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"linknest/server/internal/database"
	"linknest/server/internal/storage"

	_ "github.com/mattn/go-sqlite3"
)

func TestUploadLifecycle(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "linknest-file-test-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	db := openTestDB(t, filepath.Join(tempDir, "linknest.db"))
	defer db.Close()

	userID := seedUser(t, db)
	seedDevice(t, db, userID, "device-a")

	service := NewService(db, storage.Local{
		RootDir:  filepath.Join(tempDir, "storage"),
		ChunkDir: filepath.Join(tempDir, "chunks"),
	})

	payload := []byte("hello-linknest-upload")
	filePath := filepath.Join(tempDir, "payload.txt")
	if err := ioutil.WriteFile(filePath, payload, 0644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	fileHash, err := ComputeSHA256FromFile(filePath)
	if err != nil {
		t.Fatalf("compute file hash: %v", err)
	}

	ctx := context.Background()
	initResult, err := service.InitUpload(ctx, userID, InitUploadInput{
		DeviceID:    "device-a",
		FileName:    "payload.txt",
		FileSize:    int64(len(payload)),
		FileHash:    fileHash,
		ChunkSize:   4,
		TotalChunks: 6,
	})
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	if len(initResult.MissingChunks) != 6 {
		t.Fatalf("expected 6 missing chunks, got %d", len(initResult.MissingChunks))
	}

	for index := 0; index < 6; index++ {
		chunk, err := ReadChunk(filePath, int64(index)*4, 4)
		if err != nil {
			t.Fatalf("read chunk %d: %v", index, err)
		}
		chunkHash := sha256Hex(chunk)
		if _, err := service.UploadChunk(ctx, userID, initResult.UploadID, index, chunkHash, strings.NewReader(string(chunk))); err != nil {
			t.Fatalf("upload chunk %d: %v", index, err)
		}
	}

	completeResult, err := service.CompleteUpload(ctx, userID, initResult.UploadID)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	if completeResult.Status != "completed" {
		t.Fatalf("unexpected complete status: %s", completeResult.Status)
	}

	record, err := service.OpenDownload(ctx, userID, initResult.FileID)
	if err != nil {
		t.Fatalf("open download: %v", err)
	}
	if record.Status != "available" {
		t.Fatalf("expected available file, got %s", record.Status)
	}

	merged, err := ioutil.ReadFile(record.StoragePath)
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	if string(merged) != string(payload) {
		t.Fatalf("merged payload mismatch: got %q want %q", string(merged), string(payload))
	}

	secondInit, err := service.InitUpload(ctx, userID, InitUploadInput{
		DeviceID:    "device-a",
		FileName:    "payload.txt",
		FileSize:    int64(len(payload)),
		FileHash:    fileHash,
		ChunkSize:   4,
		TotalChunks: 6,
	})
	if err != nil {
		t.Fatalf("second init upload: %v", err)
	}
	if secondInit.Status != "available" {
		t.Fatalf("expected available status on second init, got %s", secondInit.Status)
	}
}

func TestDeleteRemovesFileMetadataAndStorage(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "linknest-file-delete-test-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	db := openTestDB(t, filepath.Join(tempDir, "linknest.db"))
	defer db.Close()

	userID := seedUser(t, db)
	seedDevice(t, db, userID, "device-a")

	service := NewService(db, storage.Local{
		RootDir:  filepath.Join(tempDir, "storage"),
		ChunkDir: filepath.Join(tempDir, "chunks"),
	})

	payload := []byte("delete-me")
	filePath := filepath.Join(tempDir, "delete.txt")
	if err := ioutil.WriteFile(filePath, payload, 0644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	fileHash, err := ComputeSHA256FromFile(filePath)
	if err != nil {
		t.Fatalf("compute file hash: %v", err)
	}

	ctx := context.Background()
	initResult, err := service.InitUpload(ctx, userID, InitUploadInput{
		DeviceID:    "device-a",
		FileName:    "delete.txt",
		FileSize:    int64(len(payload)),
		FileHash:    fileHash,
		ChunkSize:   int64(len(payload)),
		TotalChunks: 1,
	})
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	chunkHash := sha256Hex(payload)
	if _, err := service.UploadChunk(ctx, userID, initResult.UploadID, 0, chunkHash, strings.NewReader(string(payload))); err != nil {
		t.Fatalf("upload chunk: %v", err)
	}

	if _, err := service.CompleteUpload(ctx, userID, initResult.UploadID); err != nil {
		t.Fatalf("complete upload: %v", err)
	}

	record, err := service.OpenDownload(ctx, userID, initResult.FileID)
	if err != nil {
		t.Fatalf("open download before delete: %v", err)
	}
	if _, err := os.Stat(record.StoragePath); err != nil {
		t.Fatalf("stat storage before delete: %v", err)
	}

	if err := service.Delete(ctx, userID, initResult.FileID); err != nil {
		t.Fatalf("delete file: %v", err)
	}

	items, err := service.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("list files after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no visible files after delete, got %d", len(items))
	}

	if _, err := service.OpenDownload(ctx, userID, initResult.FileID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("expected ErrFileNotFound after delete, got %v", err)
	}

	if _, err := os.Stat(record.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("expected storage file removed, got err=%v", err)
	}

	var uploadTaskCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM upload_tasks WHERE file_id = ?`, initResult.FileID).Scan(&uploadTaskCount); err != nil {
		t.Fatalf("count upload tasks: %v", err)
	}
	if uploadTaskCount != 0 {
		t.Fatalf("expected upload tasks removed, got %d", uploadTaskCount)
	}
}

func openTestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := database.RunMigrations(db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func seedUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	result, err := db.Exec(`
INSERT INTO users (username, email, password_hash)
VALUES ('tester', 'tester@example.com', 'hash')
`)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("seed user id: %v", err)
	}
	return userID
}

func seedDevice(t *testing.T, db *sql.DB, userID int64, deviceID string) {
	t.Helper()

	if _, err := db.Exec(`
INSERT INTO devices (user_id, device_id, device_name, device_type, client_version, status)
VALUES (?, ?, 'demo-device', 'linux', '0.1.0', 'offline')
`, userID, deviceID); err != nil {
		t.Fatalf("seed device: %v", err)
	}
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
