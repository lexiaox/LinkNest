ALTER TABLE devices ADD COLUMN p2p_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN p2p_port INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN p2p_protocol TEXT;
ALTER TABLE devices ADD COLUMN virtual_ip TEXT;
ALTER TABLE devices ADD COLUMN public_ip TEXT;
ALTER TABLE devices ADD COLUMN p2p_updated_at TEXT;

CREATE TABLE IF NOT EXISTS transfer_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    transfer_id TEXT NOT NULL UNIQUE,
    source_device_id TEXT NOT NULL,
    target_device_id TEXT NOT NULL,
    file_id TEXT,
    file_name TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_hash TEXT NOT NULL,
    chunk_size INTEGER NOT NULL,
    total_chunks INTEGER NOT NULL,
    preferred_route TEXT NOT NULL,
    actual_route TEXT,
    status TEXT NOT NULL DEFAULT 'initialized',
    selected_candidate TEXT,
    error_code TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_transfer_tasks_user_id ON transfer_tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_transfer_tasks_transfer_id ON transfer_tasks(transfer_id);
