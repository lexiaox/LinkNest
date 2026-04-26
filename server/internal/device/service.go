package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrDeviceNotFound = errors.New("device not found")

type Service struct {
	db *sql.DB
}

type Device struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type"`
	PublicKey     string `json:"public_key,omitempty"`
	LanIP         string `json:"lan_ip,omitempty"`
	Port          int    `json:"port,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
	Status        string `json:"status"`
	LastSeenAt    string `json:"last_seen_at,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type RegisterInput struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type"`
	PublicKey     string `json:"public_key"`
	ClientVersion string `json:"client_version"`
}

type Heartbeat struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type"`
	LanIP         string `json:"lan_ip"`
	Port          int    `json:"port"`
	ClientVersion string `json:"client_version"`
	Timestamp     string `json:"timestamp"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) RegisterOrUpdate(ctx context.Context, userID int64, input RegisterInput) (Device, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.DeviceName = strings.TrimSpace(input.DeviceName)
	input.DeviceType = strings.TrimSpace(input.DeviceType)
	input.ClientVersion = strings.TrimSpace(input.ClientVersion)

	if input.DeviceID == "" || input.DeviceName == "" || input.DeviceType == "" {
		return Device{}, errors.New("device_id, device_name and device_type are required")
	}

	existing, err := s.findByDeviceID(ctx, userID, input.DeviceID)
	if err != nil && err != sql.ErrNoRows {
		return Device{}, err
	}

	if err == sql.ErrNoRows {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO devices (
	user_id, device_id, device_name, device_type, public_key, client_version, status
) VALUES (?, ?, ?, ?, ?, ?, 'offline')
`, userID, input.DeviceID, input.DeviceName, input.DeviceType, input.PublicKey, input.ClientVersion)
		if err != nil {
			return Device{}, fmt.Errorf("insert device: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
UPDATE devices
SET device_name = ?, device_type = ?, public_key = ?, client_version = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, input.DeviceName, input.DeviceType, input.PublicKey, input.ClientVersion, existing.ID)
		if err != nil {
			return Device{}, fmt.Errorf("update device: %w", err)
		}
	}

	updated, err := s.findByDeviceID(ctx, userID, input.DeviceID)
	if err != nil {
		return Device{}, err
	}
	return updated, nil
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]Device, error) {
	return s.listByUserQuery(ctx, userID, "")
}

func (s *Service) ListOnlineByUser(ctx context.Context, userID int64) ([]Device, error) {
	return s.listByUserQuery(ctx, userID, "online")
}

func (s *Service) listByUserQuery(ctx context.Context, userID int64, status string) ([]Device, error) {
	query := `
SELECT id, user_id, device_id, device_name, device_type, COALESCE(public_key, ''), COALESCE(lan_ip, ''), COALESCE(port, 0),
       COALESCE(client_version, ''), status, COALESCE(last_seen_at, ''), created_at, updated_at
FROM devices
WHERE user_id = ?`
	args := []interface{}{userID}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += "\nORDER BY updated_at DESC, id DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var item Device
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.DeviceID,
			&item.DeviceName,
			&item.DeviceType,
			&item.PublicKey,
			&item.LanIP,
			&item.Port,
			&item.ClientVersion,
			&item.Status,
			&item.LastSeenAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

func (s *Service) UpdateHeartbeat(ctx context.Context, userID int64, hb Heartbeat) error {
	if strings.TrimSpace(hb.DeviceID) == "" {
		return errors.New("device_id is required")
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE devices
SET device_name = CASE WHEN ? = '' THEN device_name ELSE ? END,
    device_type = CASE WHEN ? = '' THEN device_type ELSE ? END,
    lan_ip = ?,
    port = ?,
    client_version = CASE WHEN ? = '' THEN client_version ELSE ? END,
    status = 'online',
    last_seen_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND device_id = ?
`, hb.DeviceName, hb.DeviceName, hb.DeviceType, hb.DeviceType, hb.LanIP, hb.Port, hb.ClientVersion, hb.ClientVersion, userID, hb.DeviceID)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows: %w", err)
	}
	if affected == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *Service) MarkExpiredOffline(ctx context.Context, threshold time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE devices
SET status = 'offline', updated_at = CURRENT_TIMESTAMP
WHERE status <> 'offline'
  AND last_seen_at IS NOT NULL
  AND last_seen_at <> ''
  AND last_seen_at < ?
`, threshold.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("mark devices offline: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read affected rows: %w", err)
	}
	return affected, nil
}

func (s *Service) findByDeviceID(ctx context.Context, userID int64, deviceID string) (Device, error) {
	var item Device
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, device_id, device_name, device_type, COALESCE(public_key, ''), COALESCE(lan_ip, ''), COALESCE(port, 0),
       COALESCE(client_version, ''), status, COALESCE(last_seen_at, ''), created_at, updated_at
FROM devices
WHERE user_id = ? AND device_id = ?
`, userID, deviceID).Scan(
		&item.ID,
		&item.UserID,
		&item.DeviceID,
		&item.DeviceName,
		&item.DeviceType,
		&item.PublicKey,
		&item.LanIP,
		&item.Port,
		&item.ClientVersion,
		&item.Status,
		&item.LastSeenAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Device{}, sql.ErrNoRows
		}
		return Device{}, fmt.Errorf("find device: %w", err)
	}
	return item, nil
}
