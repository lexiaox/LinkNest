package device

import (
	"context"
	"database/sql"
	"io/ioutil"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestListOnlineByUserFiltersAtQueryLevel(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, migration := range []string{"001_init.sql", "002_v2_p2p_transfers.sql"} {
		raw, err := ioutil.ReadFile(filepath.Join("..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if _, err := db.Exec(string(raw)); err != nil {
			t.Fatalf("run migration %s: %v", migration, err)
		}
	}

	if _, err := db.Exec(`
INSERT INTO users (id, username, password_hash) VALUES
	(1, 'alice', 'hash'),
	(2, 'bob', 'hash');
INSERT INTO devices (user_id, device_id, device_name, device_type, status) VALUES
	(1, 'online-a', 'Online A', 'cli', 'online'),
	(1, 'offline-a', 'Offline A', 'cli', 'offline'),
	(2, 'online-b', 'Online B', 'cli', 'online');
`); err != nil {
		t.Fatalf("seed devices: %v", err)
	}

	items, err := NewService(db).ListOnlineByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListOnlineByUser() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListOnlineByUser) = %d, want 1", len(items))
	}
	if items[0].DeviceID != "online-a" || items[0].Status != "online" {
		t.Fatalf("ListOnlineByUser() = %#v, want only alice's online device", items)
	}
}
