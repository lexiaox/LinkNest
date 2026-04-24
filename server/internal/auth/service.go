package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"linknest/server/internal/config"
	"linknest/server/internal/storage"

	jwt "github.com/dgrijalva/jwt-go"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrBadCredentials = errors.New("bad credentials")
	ErrInvalidToken   = errors.New("invalid token")
	ErrUserExists     = errors.New("user already exists")
)

type Service struct {
	db        *sql.DB
	jwtSecret []byte
	tokenTTL  time.Duration
	storage   storage.Local
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type RegisterInput struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResult struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type DeleteAccountInput struct {
	Password string `json:"password"`
}

type DeleteAccountResult struct {
	Deleted bool `json:"deleted"`
	User    User `json:"user"`
}

type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.StandardClaims
}

func NewService(db *sql.DB, cfg config.AuthConfig, localStorage storage.Local) *Service {
	return &Service{
		db:        db,
		jwtSecret: []byte(cfg.JWTSecret),
		tokenTTL:  cfg.TokenTTL(),
		storage:   localStorage,
	}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (AuthResult, error) {
	username := strings.TrimSpace(input.Username)
	email := strings.TrimSpace(input.Email)
	password := strings.TrimSpace(input.Password)

	if username == "" || password == "" {
		return AuthResult{}, errors.New("username and password are required")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO users (username, email, password_hash)
VALUES (?, NULLIF(?, ''), ?)
`, username, email, string(passwordHash))
	if err != nil {
		if isUniqueViolation(err) {
			return AuthResult{}, ErrUserExists
		}
		return AuthResult{}, fmt.Errorf("insert user: %w", err)
	}

	userID, err := res.LastInsertId()
	if err != nil {
		return AuthResult{}, fmt.Errorf("read inserted user id: %w", err)
	}

	user, err := s.CurrentUser(ctx, userID)
	if err != nil {
		return AuthResult{}, err
	}

	token, err := s.issueToken(user)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		Token: token,
		User:  user,
	}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (AuthResult, error) {
	username := strings.TrimSpace(input.Username)
	password := strings.TrimSpace(input.Password)

	if username == "" || password == "" {
		return AuthResult{}, ErrBadCredentials
	}

	var (
		user         User
		passwordHash string
	)

	err := s.db.QueryRowContext(ctx, `
SELECT id, username, COALESCE(email, ''), password_hash, created_at, updated_at
FROM users
WHERE username = ?
`, username).Scan(&user.ID, &user.Username, &user.Email, &passwordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return AuthResult{}, ErrBadCredentials
		}
		return AuthResult{}, fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return AuthResult{}, ErrBadCredentials
	}

	token, err := s.issueToken(user)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		Token: token,
		User:  user,
	}, nil
}

func (s *Service) CurrentUser(ctx context.Context, userID int64) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, COALESCE(email, ''), created_at, updated_at
FROM users
WHERE id = ?
`, userID).Scan(&user.ID, &user.Username, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, ErrInvalidToken
		}
		return User{}, fmt.Errorf("load current user: %w", err)
	}
	return user, nil
}

func (s *Service) DeleteAccount(ctx context.Context, userID int64, input DeleteAccountInput) (DeleteAccountResult, error) {
	password := strings.TrimSpace(input.Password)
	if password == "" {
		return DeleteAccountResult{}, ErrBadCredentials
	}

	var (
		user         User
		passwordHash string
	)
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, COALESCE(email, ''), password_hash, created_at, updated_at
FROM users
WHERE id = ?
`, userID).Scan(&user.ID, &user.Username, &user.Email, &passwordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return DeleteAccountResult{}, ErrInvalidToken
		}
		return DeleteAccountResult{}, fmt.Errorf("load delete account user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return DeleteAccountResult{}, ErrBadCredentials
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DeleteAccountResult{}, fmt.Errorf("begin delete account tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
DELETE FROM file_chunks
WHERE upload_id IN (SELECT upload_id FROM upload_tasks WHERE user_id = ?)
   OR file_id IN (SELECT file_id FROM files WHERE user_id = ?)
`, userID, userID); err != nil {
		return DeleteAccountResult{}, fmt.Errorf("delete file chunks: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM users
WHERE id = ?
`, userID); err != nil {
		return DeleteAccountResult{}, fmt.Errorf("delete user: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return DeleteAccountResult{}, fmt.Errorf("commit delete account tx: %w", err)
	}

	// Best-effort filesystem cleanup: once account data is deleted in DB,
	// we should still report success even if local storage removal fails.
	_ = os.RemoveAll(s.userStorageDir(userID))
	_ = os.RemoveAll(s.userChunkDir(userID))

	return DeleteAccountResult{
		Deleted: true,
		User:    user,
	}, nil
}

func (s *Service) ParseToken(token string) (User, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(parsedToken *jwt.Token) (interface{}, error) {
		if _, ok := parsedToken.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return User{}, ErrInvalidToken
	}

	user, err := s.CurrentUser(context.Background(), claims.UserID)
	if err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return User{}, ErrInvalidToken
		}
		return User{}, err
	}
	return user, nil
}

func ExtractBearerToken(header string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", ErrInvalidToken
	}
	return strings.TrimSpace(parts[1]), nil
}

func (s *Service) issueToken(user User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: user.ID,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: now.Add(s.tokenTTL).Unix(),
			IssuedAt:  now.Unix(),
			Subject:   fmt.Sprintf("%d", user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "constraint")
}

func (s *Service) userStorageDir(userID int64) string {
	return filepath.Join(s.storage.RootDir, fmt.Sprintf("%d", userID))
}

func (s *Service) userChunkDir(userID int64) string {
	return filepath.Join(s.storage.ChunkDir, fmt.Sprintf("%d", userID))
}
