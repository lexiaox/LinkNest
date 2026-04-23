package task

import (
	"context"
	"database/sql"
	"fmt"
)

type Service struct {
	db *sql.DB
}

type UploadTask struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	UploadID       string `json:"upload_id"`
	FileID         string `json:"file_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	UploadedChunks int    `json:"uploaded_chunks"`
	Status         string `json:"status"`
	ErrorMessage   string `json:"error_message,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]UploadTask, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, upload_id, file_id, file_name, file_size, file_hash, chunk_size, total_chunks,
       uploaded_chunks, status, COALESCE(error_message, ''), created_at, updated_at
FROM upload_tasks
WHERE user_id = ?
ORDER BY created_at DESC, id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var items []UploadTask
	for rows.Next() {
		var item UploadTask
		if err := rows.Scan(
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
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return items, nil
}
