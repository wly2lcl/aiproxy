-- Create blocked_ips table for persistent IP blocking
CREATE TABLE IF NOT EXISTS blocked_ips (
    ip TEXT PRIMARY KEY,
    blocked_at DATETIME NOT NULL DEFAULT (datetime('now')),
    reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_blocked_ips_blocked_at ON blocked_ips(blocked_at);

-- Create auth_failures table for tracking authentication failures
CREATE TABLE IF NOT EXISTS auth_failures (
    ip TEXT PRIMARY KEY,
    failure_count INTEGER NOT NULL DEFAULT 1,
    first_seen DATETIME NOT NULL DEFAULT (datetime('now')),
    last_seen DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_auth_failures_first_seen ON auth_failures(first_seen);
