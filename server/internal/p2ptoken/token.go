package p2ptoken

import (
	"errors"
	"fmt"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
)

var ErrInvalidToken = errors.New("invalid p2p transfer token")

type Claims struct {
	TransferID     string `json:"transfer_id"`
	UserID         int64  `json:"user_id"`
	SourceDeviceID string `json:"source_device_id"`
	TargetDeviceID string `json:"target_device_id"`
	FileName       string `json:"file_name"`
	FileSize       int64  `json:"file_size"`
	FileHash       string `json:"file_hash"`
	ChunkSize      int64  `json:"chunk_size"`
	TotalChunks    int    `json:"total_chunks"`
	jwt.StandardClaims
}

type Service struct {
	secret []byte
	ttl    time.Duration
}

type IssueInput struct {
	TransferID     string
	UserID         int64
	SourceDeviceID string
	TargetDeviceID string
	FileName       string
	FileSize       int64
	FileHash       string
	ChunkSize      int64
	TotalChunks    int
}

func New(secret []byte, ttl time.Duration) *Service {
	return &Service{secret: secret, ttl: ttl}
}

func (s *Service) Issue(input IssueInput) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.ttl)
	claims := Claims{
		TransferID:     strings.TrimSpace(input.TransferID),
		UserID:         input.UserID,
		SourceDeviceID: strings.TrimSpace(input.SourceDeviceID),
		TargetDeviceID: strings.TrimSpace(input.TargetDeviceID),
		FileName:       strings.TrimSpace(input.FileName),
		FileSize:       input.FileSize,
		FileHash:       strings.TrimSpace(input.FileHash),
		ChunkSize:      input.ChunkSize,
		TotalChunks:    input.TotalChunks,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expiresAt.Unix(),
			IssuedAt:  now.Unix(),
			Subject:   strings.TrimSpace(input.TransferID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign p2p transfer token: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *Service) Parse(token string) (Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(strings.TrimSpace(token), claims, func(parsedToken *jwt.Token) (interface{}, error) {
		if _, ok := parsedToken.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !parsed.Valid {
		return Claims{}, ErrInvalidToken
	}
	return *claims, nil
}
