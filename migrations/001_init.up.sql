PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

CREATE TABLE providers (
    id TEXT PRIMARY KEY,
    api_base TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    config TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id TEXT NOT NULL,
    model_name TEXT NOT NULL,
    display_name TEXT,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(provider_id, model_name),
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 0,
    last_used_at DATETIME,
    last_error TEXT,
    last_error_at DATETIME,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

CREATE TABLE account_limits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id TEXT NOT NULL,
    limit_type TEXT NOT NULL,
    max_value INTEGER NOT NULL,
    current_value INTEGER NOT NULL DEFAULT 0,
    window_start DATETIME NOT NULL,
    window_end DATETIME NOT NULL,
    last_updated DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, limit_type, window_start),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

CREATE TABLE token_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    is_streaming INTEGER NOT NULL DEFAULT 0,
    is_estimated INTEGER NOT NULL DEFAULT 0,
    estimated_at DATETIME,
    corrected_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

CREATE TABLE provider_stats (
    provider_id TEXT PRIMARY KEY,
    total_requests INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    total_errors INTEGER NOT NULL DEFAULT 0,
    last_request_at DATETIME,
    last_error_at DATETIME,
    daily_tokens_used INTEGER NOT NULL DEFAULT 0,
    monthly_tokens_used INTEGER NOT NULL DEFAULT 0,
    last_daily_reset DATETIME,
    last_monthly_reset DATETIME,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);

CREATE TABLE config_state (
    key TEXT PRIMARY KEY,
    value TEXT,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_models_provider_id ON models(provider_id);
CREATE INDEX idx_models_model_name ON models(model_name);
CREATE INDEX idx_accounts_provider_id ON accounts(provider_id);
CREATE INDEX idx_accounts_is_enabled ON accounts(is_enabled);
CREATE INDEX idx_accounts_priority ON accounts(priority);
CREATE INDEX idx_accounts_last_used_at ON accounts(last_used_at);
CREATE INDEX idx_account_limits_account_id ON account_limits(account_id);
CREATE INDEX idx_account_limits_limit_type ON account_limits(limit_type);
CREATE INDEX idx_account_limits_window_start ON account_limits(window_start);
CREATE INDEX idx_account_limits_window_end ON account_limits(window_end);
CREATE INDEX idx_token_usage_account_id ON token_usage(account_id);
CREATE INDEX idx_token_usage_provider_id ON token_usage(provider_id);
CREATE INDEX idx_token_usage_request_id ON token_usage(request_id);
CREATE INDEX idx_token_usage_created_at ON token_usage(created_at);
CREATE INDEX idx_token_usage_model ON token_usage(model);
CREATE INDEX idx_provider_stats_provider_id ON provider_stats(provider_id);

CREATE VIEW v_active_accounts AS
SELECT 
    a.id,
    a.provider_id,
    a.weight,
    a.priority,
    a.last_used_at,
    p.api_base,
    p.config AS provider_config,
    al.limit_type,
    al.max_value,
    al.current_value,
    al.window_start,
    al.window_end
FROM accounts a
INNER JOIN providers p ON a.provider_id = p.id
LEFT JOIN account_limits al ON a.id = al.account_id 
    AND al.window_start <= datetime('now') 
    AND al.window_end > datetime('now')
WHERE a.is_enabled = 1 
    AND p.is_enabled = 1
    AND (al.max_value IS NULL OR al.current_value < al.max_value);

CREATE VIEW v_usage_summary_24h AS
SELECT 
    account_id,
    provider_id,
    model,
    COUNT(*) AS request_count,
    SUM(prompt_tokens) AS total_prompt_tokens,
    SUM(completion_tokens) AS total_completion_tokens,
    SUM(total_tokens) AS total_tokens,
    SUM(CASE WHEN is_streaming = 1 THEN 1 ELSE 0 END) AS streaming_count,
    SUM(CASE WHEN is_estimated = 1 THEN 1 ELSE 0 END) AS estimated_count
FROM token_usage
WHERE created_at >= datetime('now', '-24 hours')
GROUP BY account_id, provider_id, model;

CREATE TRIGGER update_timestamp
AFTER UPDATE ON accounts
FOR EACH ROW
BEGIN
    UPDATE accounts SET updated_at = datetime('now') WHERE id = NEW.id;
END;

CREATE TRIGGER update_provider_stats
AFTER INSERT ON token_usage
FOR EACH ROW
BEGIN
    INSERT INTO provider_stats (provider_id, total_requests, total_tokens, last_request_at, daily_tokens_used, monthly_tokens_used, updated_at)
    VALUES (NEW.provider_id, 1, NEW.total_tokens, datetime('now'), NEW.total_tokens, NEW.total_tokens, datetime('now'))
    ON CONFLICT(provider_id) DO UPDATE SET
        total_requests = total_requests + 1,
        total_tokens = total_tokens + NEW.total_tokens,
        last_request_at = datetime('now'),
        daily_tokens_used = daily_tokens_used + NEW.total_tokens,
        monthly_tokens_used = monthly_tokens_used + NEW.total_tokens,
        updated_at = datetime('now');
END;