package p2ptoken

import (
	"testing"
	"time"
)

func TestIssueAndParseToken(t *testing.T) {
	service := New([]byte("secret"), time.Minute)
	token, expiresAt, err := service.Issue(IssueInput{
		TransferID:     "transfer-1",
		UserID:         42,
		SourceDeviceID: "source",
		TargetDeviceID: "target",
		FileName:       "demo.bin",
		FileSize:       12,
		FileHash:       "abc",
		ChunkSize:      4,
		TotalChunks:    3,
	})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatalf("expiresAt = %v, want future time", expiresAt)
	}

	claims, err := service.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.TransferID != "transfer-1" || claims.UserID != 42 || claims.TargetDeviceID != "target" {
		t.Fatalf("claims = %#v, want transfer/user/target metadata", claims)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	service := New([]byte("secret"), -time.Second)
	token, _, err := service.Issue(IssueInput{
		TransferID:     "transfer-1",
		UserID:         42,
		SourceDeviceID: "source",
		TargetDeviceID: "target",
		FileName:       "demo.bin",
		FileSize:       12,
		FileHash:       "abc",
		ChunkSize:      4,
		TotalChunks:    3,
	})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := service.Parse(token); err != ErrInvalidToken {
		t.Fatalf("Parse() error = %v, want ErrInvalidToken", err)
	}
}
