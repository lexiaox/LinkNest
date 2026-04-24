package auth

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"linknest/server/internal/config"
	"linknest/server/internal/database"
	"linknest/server/internal/storage"

	_ "github.com/mattn/go-sqlite3"
)

func TestDeleteAccountRemovesUserDataAndStorage(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "linknest-auth-delete-test-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	db := openAuthTestDB(t, filepath.Join(tempDir, "linknest.db"))
	defer db.Close()

	userID := seedAuthUser(t, db, "delete-user", "delete@example.com", "password")
	seedAuthData(t, db, userID, tempDir)

	service := NewService(db, config.AuthConfig{
		JWTSecret:     "test-secret",
		TokenTTLHours: 24,
	}, storage.Local{
		RootDir:  filepath.Join(tempDir, "storage"),
		ChunkDir: filepath.Join(tempDir, "chunks"),
	})

	result, err := service.DeleteAccount(context.Background(), userID, DeleteAccountInput{
		Password: "password",
	})
	if err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if !result.Deleted || result.User.ID != userID {
		t.Fatalf("unexpected delete result: %+v", result)
	}

	assertCount(t, db, "SELECT COUNT(*) FROM users WHERE id = ?", userID, 0)
	assertCount(t, db, "SELECT COUNT(*) FROM devices WHERE user_id = ?", userID, 0)
	assertCount(t, db, "SELECT COUNT(*) FROM files WHERE user_id = ?", userID, 0)
	assertCount(t, db, "SELECT COUNT(*) FROM upload_tasks WHERE user_id = ?", userID, 0)
	assertCount(t, db, "SELECT COUNT(*) FROM file_chunks", nil, 0)

	userDir := fmt.Sprintf("%d", userID)
	if _, err := os.Stat(filepath.Join(tempDir, "storage", userDir)); !os.IsNotExist(err) {
		t.Fatalf("expected storage dir removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "chunks", userDir)); !os.IsNotExist(err) {
		t.Fatalf("expected chunk dir removed, err=%v", err)
	}
}

func openAuthTestDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := database.RunMigrations(db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func seedAuthUser(t *testing.T, db *sql.DB, username string, email string, password string) int64 {
	t.Helper()

	service := NewService(db, config.AuthConfig{
		JWTSecret:     "test-secret",
		TokenTTLHours: 24,
	}, storage.Local{})
	result, err := service.Register(context.Background(), RegisterInput{
		Username: username,
		Email:    email,
		Password: password,
	})
	if err != nil {
		t.Fatalf("seed auth user: %v", err)
	}
	return result.User.ID
}

func seedAuthData(t *testing.T, db *sql.DB, userID int64, tempDir string) {
	t.Helper()

	userDir := fmt.Sprintf("%d", userID)
	storageDir := filepath.Join(tempDir, "storage", userDir)
	chunkDir := filepath.Join(tempDir, "chunks", userDir, "upload-1")
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("mkdir storage dir: %v", err)
	}
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		t.Fatalf("mkdir chunk dir: %v", err)
	}

	finalPath := filepath.Join(storageDir, "file-1")
	chunkPath := filepath.Join(chunkDir, "0.part")
	if err := ioutil.WriteFile(finalPath, []byte("final"), 0644); err != nil {
		t.Fatalf("write final file: %v", err)
	}
	if err := ioutil.WriteFile(chunkPath, []byte("chunk"), 0644); err != nil {
		t.Fatalf("write chunk file: %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO devices (user_id, device_id, device_name, device_type, client_version, status)
VALUES (?, 'device-1', 'demo-device', 'linux', '0.1.0', 'offline')
`, userID); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO files (user_id, file_id, file_name, file_size, file_hash, uploader_device_id, storage_path, status)
VALUES (?, 'file-1', 'demo.txt', 5, 'hash', 'device-1', ?, 'available')
`, userID, finalPath); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO upload_tasks (user_id, upload_id, file_id, file_name, file_size, file_hash, chunk_size, total_chunks, uploaded_chunks, status)
VALUES (?, 'upload-1', 'file-1', 'demo.txt', 5, 'hash', 5, 1, 1, 'completed')
`, userID); err != nil {
		t.Fatalf("seed upload task: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO file_chunks (upload_id, file_id, chunk_index, chunk_hash, chunk_size, storage_path, status)
VALUES ('upload-1', 'file-1', 0, 'hash', 5, ?, 'uploaded')
`, chunkPath); err != nil {
		t.Fatalf("seed file chunk: %v", err)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, arg interface{}, want int) {
	t.Helper()

	var got int
	var err error
	if arg == nil {
		err = db.QueryRow(query).Scan(&got)
	} else {
		err = db.QueryRow(query, arg).Scan(&got)
	}
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected count for %q: got %d want %d", query, got, want)
	}
}
