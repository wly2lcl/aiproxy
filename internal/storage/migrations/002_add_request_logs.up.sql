-- Add fields for request logging and performance tracking
ALTER TABLE token_usage ADD COLUMN status INTEGER NOT NULL DEFAULT 200;
ALTER TABLE token_usage ADD COLUMN ttft_ms REAL;
ALTER TABLE token_usage ADD COLUMN latency_ms REAL;
ALTER TABLE token_usage ADD COLUMN error_type TEXT;

-- Create request_logs table for recent request tracking
CREATE TABLE IF NOT EXISTS request_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    status INTEGER NOT NULL DEFAULT 200,
    tokens INTEGER NOT NULL DEFAULT 0,
    ttft_ms REAL,
    latency_ms REAL,
    error_type TEXT,
    is_streaming INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

-- Create indexes for request_logs
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_account_id ON request_logs(account_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_provider_id ON request_logs(provider_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_status ON request_logs(status);