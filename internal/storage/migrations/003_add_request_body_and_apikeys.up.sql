-- Add request/response body storage for detailed logging
ALTER TABLE request_logs ADD COLUMN request_body TEXT;
ALTER TABLE request_logs ADD COLUMN response_body TEXT;
ALTER TABLE request_logs ADD COLUMN error_message TEXT;

-- Create api_keys table for public API key management
CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_used_at DATETIME,
    request_count INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_is_enabled ON api_keys(is_enabled);