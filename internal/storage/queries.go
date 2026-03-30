package storage

const (
	upsertProviderQuery = `
INSERT INTO providers (id, api_base, is_default, is_enabled, config)
VALUES (:id, :api_base, :is_default, :is_enabled, :config)
ON CONFLICT(id) DO UPDATE SET
	api_base = excluded.api_base,
	is_default = excluded.is_default,
	is_enabled = excluded.is_enabled,
	config = excluded.config,
	updated_at = datetime('now')`

	getProviderQuery = `
SELECT id, api_base, is_default, is_enabled, config, created_at, updated_at
FROM providers
WHERE id = ?`

	listProvidersQuery = `
SELECT id, api_base, is_default, is_enabled, config, created_at, updated_at
FROM providers
ORDER BY id`

	getProviderModelsQuery = `
SELECT model_name, display_name, is_enabled
FROM models
WHERE provider_id = ?`

	upsertAccountQuery = `
INSERT INTO accounts (id, provider_id, api_key_hash, weight, is_enabled, priority)
VALUES (:id, :provider_id, :api_key_hash, :weight, :is_enabled, :priority)
ON CONFLICT(id) DO UPDATE SET
	provider_id = excluded.provider_id,
	api_key_hash = excluded.api_key_hash,
	weight = excluded.weight,
	is_enabled = excluded.is_enabled,
	priority = excluded.priority,
	updated_at = datetime('now')`

	getAccountQuery = `
SELECT id, provider_id, api_key_hash, weight, is_enabled, priority, last_used_at, last_error, last_error_at, consecutive_failures, created_at, updated_at
FROM accounts
WHERE id = ?`

	listAccountsQuery = `
SELECT id, provider_id, api_key_hash, weight, is_enabled, priority, last_used_at, last_error, last_error_at, consecutive_failures, created_at, updated_at
FROM accounts
WHERE provider_id = ?
ORDER BY priority DESC, weight DESC, id`

	getAllAccountLastUsedQuery = `
SELECT id, last_used_at
FROM accounts`

	getRateLimitQuery = `
SELECT limit_type, max_value, current_value, window_start, window_end
FROM account_limits
WHERE account_id = ? AND limit_type = ? AND window_start <= datetime('now') AND window_end > datetime('now')`

	incrementRateLimitQuery = `
INSERT INTO account_limits (account_id, limit_type, max_value, current_value, window_start, window_end)
VALUES (:account_id, :limit_type, :max_value, :current_value, :window_start, :window_end)
ON CONFLICT(account_id, limit_type, window_start) DO UPDATE SET
	current_value = current_value + :delta,
	last_updated = datetime('now')`

	resetRateLimitQuery = `
DELETE FROM account_limits
WHERE account_id = ? AND limit_type = ?`

	recordTokenUsageQuery = `
INSERT INTO token_usage (request_id, account_id, provider_id, model, prompt_tokens, completion_tokens, total_tokens, is_streaming, is_estimated, estimated_at, corrected_at)
VALUES (:request_id, :account_id, :provider_id, :model, :prompt_tokens, :completion_tokens, :total_tokens, :is_streaming, :is_estimated, :estimated_at, :corrected_at)`

	getTokenUsageQuery = `
SELECT 
	COUNT(*) AS request_count,
	SUM(prompt_tokens) AS total_prompt_tokens,
	SUM(completion_tokens) AS total_completion_tokens,
	SUM(total_tokens) AS total_tokens,
	SUM(CASE WHEN is_streaming = 1 THEN 1 ELSE 0 END) AS streaming_count,
	SUM(CASE WHEN is_estimated = 1 THEN 1 ELSE 0 END) AS estimated_count
FROM token_usage
WHERE account_id = ? AND created_at >= ?`

	updateAccountLastUsedQuery = `
UPDATE accounts SET last_used_at = datetime('now'), updated_at = datetime('now')
WHERE id = ?`

	getSchemaVersionQuery = `
SELECT value FROM config_state WHERE key = 'schema_version'`

	setSchemaVersionQuery = `
INSERT INTO config_state (key, value) VALUES ('schema_version', ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')`

	getAllRateLimitsQuery = `
SELECT limit_type, max_value, current_value, window_start, window_end
FROM account_limits
WHERE account_id = ? AND window_start <= datetime('now') AND window_end > datetime('now')
ORDER BY limit_type`

	getRecentLogsQuery = `
SELECT request_id, account_id, provider_id, model, status, tokens, ttft_ms, latency_ms, error_type, created_at, is_streaming
FROM request_logs
ORDER BY created_at DESC
LIMIT ?`

	recordRequestLogQuery = `
INSERT INTO request_logs (request_id, account_id, provider_id, model, status, tokens, ttft_ms, latency_ms, error_type, is_streaming)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
)
