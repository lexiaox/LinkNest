package storage

import (
	"fmt"
	"path/filepath"
)

type Local struct {
	RootDir  string
	ChunkDir string
}

func (l Local) FinalPath(userID int64, fileID string) string {
	return filepath.Join(l.RootDir, fmt.Sprintf("%d", userID), fileID)
}

func (l Local) ChunkPath(userID int64, uploadID string, chunkIndex int) string {
	return filepath.Join(l.ChunkDir, fmt.Sprintf("%d", userID), uploadID, fmt.Sprintf("%d.part", chunkIndex))
}
