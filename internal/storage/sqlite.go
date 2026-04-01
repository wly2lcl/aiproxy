package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wangluyao/aiproxy/internal/config"
	"github.com/wangluyao/aiproxy/internal/domain"

	_ "modernc.org/sqlite"
)

type SQLite struct {
	db *sql.DB
}

func NewSQLite(cfg *config.DatabaseConfig) (*SQLite, error) {
	busyTimeout := cfg.BusyTimeout
	if busyTimeout <= 0 {
		busyTimeout = 5000
	}
	journalMode := cfg.JournalMode
	if journalMode == "" {
		journalMode = "WAL"
	}

	dsn := cfg.Path
	prefix := "?"
	if strings.Contains(dsn, "?") {
		prefix = "&"
	}
	dsn += fmt.Sprintf("%s_pragma=busy_timeout(%d)&_pragma=journal_mode(%s)", prefix, busyTimeout, journalMode)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings suitable for WAL mode
	// Multiple readers and 1 concurrent writer with timeout on contention
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = maxOpen
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	// 定期回收连接，防止长时间运行后连接积累导致文件锁问题
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) UpsertProvider(ctx context.Context, provider *domain.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	if err := provider.Validate(); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, upsertProviderQuery,
		sql.Named("id", provider.ID),
		sql.Named("api_base", provider.APIBase),
		sql.Named("is_default", provider.IsDefault),
		sql.Named("is_enabled", provider.IsEnabled),
		sql.Named("config", nil),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert provider: %w", err)
	}

	return nil
}

func (s *SQLite) GetProvider(ctx context.Context, id string) (*domain.Provider, error) {
	if id == "" {
		return nil, fmt.Errorf("provider id cannot be empty")
	}

	var provider domain.Provider
	var config *string
	err := s.db.QueryRowContext(ctx, getProviderQuery, id).Scan(
		&provider.ID,
		&provider.APIBase,
		&provider.IsDefault,
		&provider.IsEnabled,
		&config,
		&sql.NullTime{},
		&sql.NullTime{},
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewDomainError(domain.ErrCodeProviderNotFound, fmt.Sprintf("provider %s not found", id))
		}
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	provider.Models = []string{}

	return &provider, nil
}

func (s *SQLite) ListProviders(ctx context.Context) ([]*domain.Provider, error) {
	rows, err := s.db.QueryContext(ctx, listProvidersQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}
	defer rows.Close()

	var providers []*domain.Provider
	for rows.Next() {
		var provider domain.Provider
		var config *string
		err := rows.Scan(
			&provider.ID,
			&provider.APIBase,
			&provider.IsDefault,
			&provider.IsEnabled,
			&config,
			&sql.NullTime{},
			&sql.NullTime{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		provider.Models = []string{}
		providers = append(providers, &provider)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating providers: %w", err)
	}

	return providers, nil
}

func (s *SQLite) UpsertAccount(ctx context.Context, account *domain.Account) error {
	if account == nil {
		return fmt.Errorf("account cannot be nil")
	}

	if err := account.Validate(); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, upsertAccountQuery,
		sql.Named("id", account.ID),
		sql.Named("provider_id", account.ProviderID),
		sql.Named("api_key_hash", account.APIKeyHash),
		sql.Named("weight", account.Weight),
		sql.Named("is_enabled", account.IsEnabled),
		sql.Named("priority", account.Priority),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert account: %w", err)
	}

	return nil
}

func (s *SQLite) GetAccount(ctx context.Context, id string) (*domain.Account, error) {
	if id == "" {
		return nil, fmt.Errorf("account id cannot be empty")
	}

	var account domain.Account
	var lastUsedAt, lastErrorAt sql.NullTime
	var lastError sql.NullString
	var consecutiveFailures sql.NullInt64
	err := s.db.QueryRowContext(ctx, getAccountQuery, id).Scan(
		&account.ID,
		&account.ProviderID,
		&account.APIKeyHash,
		&account.Weight,
		&account.IsEnabled,
		&account.Priority,
		&lastUsedAt,
		&lastError,
		&lastErrorAt,
		&consecutiveFailures,
		&sql.NullTime{},
		&sql.NullTime{},
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewDomainError(domain.ErrCodeAccountNotFound, fmt.Sprintf("account %s not found", id))
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &account, nil
}

func (s *SQLite) ListAccounts(ctx context.Context, providerID string) ([]*domain.Account, error) {
	rows, err := s.db.QueryContext(ctx, listAccountsQuery, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*domain.Account
	for rows.Next() {
		var account domain.Account
		var lastUsedAt, lastErrorAt sql.NullTime
		var lastError sql.NullString
		var consecutiveFailures sql.NullInt64
		err := rows.Scan(
			&account.ID,
			&account.ProviderID,
			&account.APIKeyHash,
			&account.Weight,
			&account.IsEnabled,
			&account.Priority,
			&lastUsedAt,
			&lastError,
			&lastErrorAt,
			&consecutiveFailures,
			&sql.NullTime{},
			&sql.NullTime{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, &account)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating accounts: %w", err)
	}

	return accounts, nil
}

func (s *SQLite) GetRateLimit(ctx context.Context, accountID string, limitType domain.LimitType) (*domain.LimitState, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id cannot be empty")
	}

	var state domain.LimitState
	var limitTypeStr string
	var windowStart, windowEnd sql.NullTime
	err := s.db.QueryRowContext(ctx, getRateLimitQuery, accountID, string(limitType)).Scan(
		&limitTypeStr,
		&state.Max,
		&state.Current,
		&windowStart,
		&windowEnd,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get rate limit: %w", err)
	}

	state.Type = domain.LimitType(limitTypeStr)
	if windowStart.Valid {
		state.WindowStart = windowStart.Time
	}
	if windowEnd.Valid {
		state.WindowEnd = windowEnd.Time
	}

	return &state, nil
}

func (s *SQLite) IncrementRateLimit(ctx context.Context, accountID string, limitType domain.LimitType, delta int) error {
	if accountID == "" {
		return fmt.Errorf("account id cannot be empty")
	}

	now := time.Now().UTC()
	windowStart := now.Truncate(time.Minute)
	var windowEnd time.Time

	switch limitType {
	case domain.LimitTypeRPM:
		windowEnd = windowStart.Add(time.Minute)
	case domain.LimitTypeDaily, domain.LimitTypeTokenDaily:
		windowEnd = windowStart.Add(24 * time.Hour)
	case domain.LimitTypeWindow5h:
		windowEnd = windowStart.Add(5 * time.Hour)
	case domain.LimitTypeMonthly, domain.LimitTypeTokenMonthly:
		windowEnd = windowStart.AddDate(0, 1, 0)
	default:
		return fmt.Errorf("unknown limit type: %s", limitType)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO account_limits (account_id, limit_type, max_value, current_value, window_start, window_end) 
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, limit_type, window_start) 
		 DO UPDATE SET current_value = current_value + excluded.current_value, last_updated = datetime('now')`,
		accountID, string(limitType), 0, delta, windowStart.Format("2006-01-02 15:04:05"), windowEnd.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert rate limit: %w", err)
	}

	return nil
}

func (s *SQLite) ResetRateLimit(ctx context.Context, accountID string, limitType domain.LimitType) error {
	if accountID == "" {
		return fmt.Errorf("account id cannot be empty")
	}

	_, err := s.db.ExecContext(ctx, resetRateLimitQuery, accountID, string(limitType))
	if err != nil {
		return fmt.Errorf("failed to reset rate limit: %w", err)
	}

	return nil
}

func (s *SQLite) UpdateAccountLastUsed(ctx context.Context, accountID string) error {
	if accountID == "" {
		return fmt.Errorf("account id cannot be empty")
	}

	_, err := s.db.ExecContext(ctx, updateAccountLastUsedQuery, accountID)
	if err != nil {
		return fmt.Errorf("failed to update account last used: %w", err)
	}

	return nil
}

func (s *SQLite) GetAllAccountLastUsed(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, getAllAccountLastUsedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get account last used: %w", err)
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var id string
		var lastUsedAt sql.NullTime
		if err := rows.Scan(&id, &lastUsedAt); err != nil {
			return nil, fmt.Errorf("failed to scan account last used: %w", err)
		}
		if lastUsedAt.Valid {
			result[id] = lastUsedAt.Time
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating account last used: %w", err)
	}

	return result, nil
}

func (s *SQLite) RecordTokenUsage(ctx context.Context, usage *TokenUsage) error {
	if usage == nil {
		return fmt.Errorf("usage cannot be nil")
	}

	_, err := s.db.ExecContext(ctx, recordTokenUsageQuery,
		sql.Named("request_id", usage.RequestID),
		sql.Named("account_id", usage.AccountID),
		sql.Named("provider_id", usage.ProviderID),
		sql.Named("model", usage.Model),
		sql.Named("prompt_tokens", usage.PromptTokens),
		sql.Named("completion_tokens", usage.CompletionTokens),
		sql.Named("total_tokens", usage.TotalTokens),
		sql.Named("is_streaming", usage.IsStreaming),
		sql.Named("is_estimated", usage.IsEstimated),
		sql.Named("estimated_at", usage.EstimatedAt),
		sql.Named("corrected_at", usage.CorrectedAt),
	)
	if err != nil {
		return fmt.Errorf("failed to record token usage: %w", err)
	}

	return nil
}

func (s *SQLite) GetTokenUsage(ctx context.Context, accountID string, since time.Time) (*TokenUsageSummary, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id cannot be empty")
	}

	var summary TokenUsageSummary
	var requestCount sql.NullInt64
	var totalPromptTokens, totalCompletionTokens, totalTokens sql.NullInt64
	var streamingCount, estimatedCount sql.NullInt64

	err := s.db.QueryRowContext(ctx, getTokenUsageQuery, accountID, since.UTC().Format("2006-01-02 15:04:05")).Scan(
		&requestCount,
		&totalPromptTokens,
		&totalCompletionTokens,
		&totalTokens,
		&streamingCount,
		&estimatedCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return &TokenUsageSummary{}, nil
		}
		return nil, fmt.Errorf("failed to get token usage: %w", err)
	}

	if requestCount.Valid {
		summary.RequestCount = int(requestCount.Int64)
	}
	if totalPromptTokens.Valid {
		summary.TotalPromptTokens = int(totalPromptTokens.Int64)
	}
	if totalCompletionTokens.Valid {
		summary.TotalCompletionTokens = int(totalCompletionTokens.Int64)
	}
	if totalTokens.Valid {
		summary.TotalTokens = int(totalTokens.Int64)
	}
	if streamingCount.Valid {
		summary.StreamingCount = int(streamingCount.Int64)
	}
	if estimatedCount.Valid {
		summary.EstimatedCount = int(estimatedCount.Int64)
	}

	return &summary, nil
}

func (s *SQLite) CleanupExpiredRateLimits(ctx context.Context) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM account_limits WHERE window_end < ?",
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired rate limits: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		slog.Info("cleaned up expired rate limits", "count", rowsAffected)
	}

	return nil
}

func (s *SQLite) GetAllRateLimits(ctx context.Context, accountID string) ([]*domain.LimitState, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id cannot be empty")
	}

	rows, err := s.db.QueryContext(ctx, getAllRateLimitsQuery, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limits: %w", err)
	}
	defer rows.Close()

	var limits []*domain.LimitState
	for rows.Next() {
		var state domain.LimitState
		var limitTypeStr string
		var windowStart, windowEnd sql.NullTime
		err := rows.Scan(
			&limitTypeStr,
			&state.Max,
			&state.Current,
			&windowStart,
			&windowEnd,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rate limit: %w", err)
		}
		state.Type = domain.LimitType(limitTypeStr)
		if windowStart.Valid {
			state.WindowStart = windowStart.Time
		}
		if windowEnd.Valid {
			state.WindowEnd = windowEnd.Time
		}
		limits = append(limits, &state)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rate limits: %w", err)
	}

	return limits, nil
}

func (s *SQLite) GetRecentLogs(ctx context.Context, limit int) ([]*RequestLog, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, getRecentLogsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent logs: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLog
	for rows.Next() {
		var log RequestLog
		var ttftMs, latencyMs sql.NullFloat64
		var errorType sql.NullString
		var isStreaming sql.NullBool
		var timestamp sql.NullTime
		err := rows.Scan(
			&log.RequestID,
			&log.AccountID,
			&log.ProviderID,
			&log.Model,
			&log.Status,
			&log.Tokens,
			&ttftMs,
			&latencyMs,
			&errorType,
			&timestamp,
			&isStreaming,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}
		if ttftMs.Valid {
			log.TTFTMs = ttftMs.Float64
		}
		if latencyMs.Valid {
			log.LatencyMs = latencyMs.Float64
		}
		if errorType.Valid {
			log.ErrorType = errorType.String
		}
		if timestamp.Valid {
			log.Timestamp = timestamp.Time
		}
		if isStreaming.Valid {
			log.IsStreaming = isStreaming.Bool
		}
		logs = append(logs, &log)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating logs: %w", err)
	}

	return logs, nil
}

func (s *SQLite) GetLogByID(ctx context.Context, requestID string) (*RequestLog, error) {
	row := s.db.QueryRowContext(ctx, getLogByIDQuery, requestID)

	var log RequestLog
	var ttftMs, latencyMs sql.NullFloat64
	var errorType, errorMessage, requestBody, responseBody sql.NullString
	var isStreaming sql.NullBool
	var timestamp sql.NullTime

	err := row.Scan(
		&log.RequestID,
		&log.AccountID,
		&log.ProviderID,
		&log.Model,
		&log.Status,
		&log.Tokens,
		&ttftMs,
		&latencyMs,
		&errorType,
		&errorMessage,
		&timestamp,
		&isStreaming,
		&requestBody,
		&responseBody,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get log by id: %w", err)
	}

	if ttftMs.Valid {
		log.TTFTMs = ttftMs.Float64
	}
	if latencyMs.Valid {
		log.LatencyMs = latencyMs.Float64
	}
	if errorType.Valid {
		log.ErrorType = errorType.String
	}
	if errorMessage.Valid {
		log.ErrorMessage = errorMessage.String
	}
	if timestamp.Valid {
		log.Timestamp = timestamp.Time
	}
	if isStreaming.Valid {
		log.IsStreaming = isStreaming.Bool
	}
	if requestBody.Valid {
		log.RequestBody = requestBody.String
	}
	if responseBody.Valid {
		log.ResponseBody = responseBody.String
	}

	return &log, nil
}

func (s *SQLite) RecordRequestLog(ctx context.Context, log *RequestLog) error {
	if log == nil {
		return fmt.Errorf("log cannot be nil")
	}

	_, err := s.db.ExecContext(ctx, recordRequestLogWithBodyQuery,
		log.RequestID,
		log.AccountID,
		log.ProviderID,
		log.Model,
		log.Status,
		log.Tokens,
		log.TTFTMs,
		log.LatencyMs,
		log.ErrorType,
		log.ErrorMessage,
		log.IsStreaming,
		log.RequestBody,
		log.ResponseBody,
	)
	if err != nil {
		return fmt.Errorf("failed to record request log: %w", err)
	}

	return nil
}

func (s *SQLite) GetRequestTimeSeries(ctx context.Context, since time.Time, interval string) ([]*TimeSeriesPoint, error) {
	var query string
	switch interval {
	case "hour":
		query = getTimeSeriesHourlyQuery
	case "day":
		query = getTimeSeriesDailyQuery
	default:
		query = getTimeSeriesHourlyQuery
	}

	rows, err := s.db.QueryContext(ctx, query, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("failed to get time series: %w", err)
	}
	defer rows.Close()

	var points []*TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		var tsStr string
		err := rows.Scan(&tsStr, &p.Count, &p.Tokens, &p.Errors)
		if err != nil {
			return nil, fmt.Errorf("failed to scan time series point: %w", err)
		}
		p.Timestamp, err = time.Parse("2006-01-02 15:04:05", tsStr)
		if err != nil {
			slog.Error("failed to parse timestamp", "timestamp", tsStr, "error", err)
		}
		points = append(points, &p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating time series: %w", err)
	}

	return points, nil
}

func (s *SQLite) GetAllAccountStats(ctx context.Context, since time.Time) ([]*AccountStats, error) {
	rows, err := s.db.QueryContext(ctx, getAllAccountStatsQuery, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("failed to get all account stats: %w", err)
	}
	defer rows.Close()

	var stats []*AccountStats
	for rows.Next() {
		var st AccountStats
		var avgLatency, avgTTFT, successRate sql.NullFloat64
		var lastUsed sql.NullString
		err := rows.Scan(&st.AccountID, &st.RequestCount, &st.ErrorCount, &st.TotalTokens, &avgLatency, &avgTTFT, &successRate, &lastUsed)
		if err != nil {
			return nil, fmt.Errorf("failed to scan account stats: %w", err)
		}
		if avgLatency.Valid {
			st.AvgLatencyMs = avgLatency.Float64
		}
		if avgTTFT.Valid {
			st.AvgTTFTMs = avgTTFT.Float64
		}
		if successRate.Valid {
			st.SuccessRate = successRate.Float64
		}
		if lastUsed.Valid && lastUsed.String != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastUsed.String); err == nil {
				st.LastUsedAt = &t
			}
		}
		stats = append(stats, &st)
	}

	return stats, nil
}

func (s *SQLite) GetModelStats(ctx context.Context, since time.Time) ([]*ModelStats, error) {
	rows, err := s.db.QueryContext(ctx, getModelStatsQuery, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("failed to get model stats: %w", err)
	}
	defer rows.Close()

	var stats []*ModelStats
	for rows.Next() {
		var st ModelStats
		var avgLatency, avgTTFT, successRate sql.NullFloat64
		err := rows.Scan(&st.Model, &st.RequestCount, &st.ErrorCount, &st.TotalTokens, &avgLatency, &avgTTFT, &successRate)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model stats: %w", err)
		}
		if avgLatency.Valid {
			st.AvgLatencyMs = avgLatency.Float64
		}
		if avgTTFT.Valid {
			st.AvgTTFTMs = avgTTFT.Float64
		}
		if successRate.Valid {
			st.SuccessRate = successRate.Float64
		}
		stats = append(stats, &st)
	}

	return stats, nil
}

func (s *SQLite) GetLatencyData(ctx context.Context, since time.Time) ([]*LatencyData, error) {
	rows, err := s.db.QueryContext(ctx, getLatencyDataQuery, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("failed to get latency data: %w", err)
	}
	defer rows.Close()

	var data []*LatencyData
	for rows.Next() {
		var d LatencyData
		var latency, ttft sql.NullFloat64
		err := rows.Scan(&latency, &ttft, &d.Status)
		if err != nil {
			return nil, fmt.Errorf("failed to scan latency data: %w", err)
		}
		if latency.Valid {
			d.LatencyMs = latency.Float64
		}
		if ttft.Valid {
			d.TTFTMs = ttft.Float64
		}
		data = append(data, &d)
	}

	return data, nil
}

func (s *SQLite) GetAccountModelStats(ctx context.Context, accountID string, since time.Time) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, getAccountModelStatsQuery, accountID, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("failed to get account model stats: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var model string
		var count int64
		if err := rows.Scan(&model, &count); err != nil {
			return nil, fmt.Errorf("failed to scan model stats: %w", err)
		}
		result[model] = count
	}

	return result, nil
}

func (s *SQLite) CreateAPIKey(ctx context.Context, keyHash, name string, expiresAt *time.Time) (int64, error) {
	var expiresStr interface{}
	if expiresAt != nil {
		expiresStr = expiresAt.UTC().Format("2006-01-02 15:04:05")
	}

	result, err := s.db.ExecContext(ctx, createAPIKeyQuery, keyHash, name, expiresStr)
	if err != nil {
		return 0, fmt.Errorf("failed to create api key: %w", err)
	}

	return result.LastInsertId()
}

func (s *SQLite) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	row := s.db.QueryRowContext(ctx, getAPIKeyByHashQuery, keyHash)

	var key APIKey
	var lastUsed, expires sql.NullTime
	err := row.Scan(
		&key.ID,
		&key.KeyHash,
		&key.Name,
		&key.IsEnabled,
		&key.CreatedAt,
		&lastUsed,
		&key.RequestCount,
		&expires,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get api key: %w", err)
	}

	if lastUsed.Valid {
		key.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		key.ExpiresAt = &expires.Time
	}

	return &key, nil
}

func (s *SQLite) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	rows, err := s.db.QueryContext(ctx, listAPIKeysQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var key APIKey
		var lastUsed, expires sql.NullTime
		err := rows.Scan(
			&key.ID,
			&key.KeyHash,
			&key.Name,
			&key.IsEnabled,
			&key.CreatedAt,
			&lastUsed,
			&key.RequestCount,
			&expires,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan api key: %w", err)
		}

		if lastUsed.Valid {
			key.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			key.ExpiresAt = &expires.Time
		}
		keys = append(keys, &key)
	}

	return keys, nil
}

func (s *SQLite) UpdateAPIKeyUsage(ctx context.Context, keyHash string) error {
	_, err := s.db.ExecContext(ctx, updateAPIKeyUsageQuery, keyHash)
	if err != nil {
		return fmt.Errorf("failed to update api key usage: %w", err)
	}
	return nil
}

func (s *SQLite) DeleteAPIKey(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, deleteAPIKeyQuery, id)
	if err != nil {
		return fmt.Errorf("failed to delete api key: %w", err)
	}
	return nil
}

func (s *SQLite) ToggleAPIKey(ctx context.Context, id int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx, toggleAPIKeyQuery, enabled, id)
	if err != nil {
		return fmt.Errorf("failed to toggle api key: %w", err)
	}
	return nil
}

func (s *SQLite) BlockIP(ctx context.Context, ip, reason string) error {
	_, err := s.db.ExecContext(ctx, blockIPQuery, ip, reason)
	if err != nil {
		return fmt.Errorf("failed to block ip: %w", err)
	}
	return nil
}

func (s *SQLite) UnblockIP(ctx context.Context, ip string) error {
	_, err := s.db.ExecContext(ctx, unblockIPQuery, ip)
	if err != nil {
		return fmt.Errorf("failed to unblock ip: %w", err)
	}
	return nil
}

func (s *SQLite) GetBlockedIPs(ctx context.Context) ([]BlockedIP, error) {
	rows, err := s.db.QueryContext(ctx, getBlockedIPsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get blocked ips: %w", err)
	}
	defer rows.Close()

	ips := make([]BlockedIP, 0)
	for rows.Next() {
		var ip BlockedIP
		var blockedAtStr string
		var reason sql.NullString
		if err := rows.Scan(&ip.IP, &blockedAtStr, &reason); err != nil {
			return nil, fmt.Errorf("failed to scan blocked ip: %w", err)
		}
		ip.BlockedAt, err = time.Parse(time.RFC3339, blockedAtStr)
		if err != nil {
			slog.Debug("failed to parse blocked_at as RFC3339, trying alternative format", "blocked_at", blockedAtStr, "error", err)
			ip.BlockedAt, err = time.Parse("2006-01-02 15:04:05", blockedAtStr)
			if err != nil {
				slog.Error("failed to parse blocked_at timestamp", "blocked_at", blockedAtStr, "error", err)
			}
		}
		if reason.Valid {
			ip.Reason = reason.String
		}
		ips = append(ips, ip)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate blocked ips: %w", err)
	}
	return ips, nil
}

func (s *SQLite) RecordAuthFailure(ctx context.Context, ip string) error {
	_, err := s.db.ExecContext(ctx, recordAuthFailureQuery, ip)
	if err != nil {
		return fmt.Errorf("failed to record auth failure: %w", err)
	}
	return nil
}

func (s *SQLite) ClearAuthFailure(ctx context.Context, ip string) error {
	_, err := s.db.ExecContext(ctx, clearAuthFailureQuery, ip)
	if err != nil {
		return fmt.Errorf("failed to clear auth failure: %w", err)
	}
	return nil
}

func (s *SQLite) GetAuthFailures(ctx context.Context) ([]AuthFailure, error) {
	rows, err := s.db.QueryContext(ctx, getAuthFailuresQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth failures: %w", err)
	}
	defer rows.Close()

	failures := make([]AuthFailure, 0)
	for rows.Next() {
		var f AuthFailure
		var firstSeenStr, lastSeenStr string
		if err := rows.Scan(&f.IP, &f.FailureCount, &firstSeenStr, &lastSeenStr); err != nil {
			return nil, fmt.Errorf("failed to scan auth failure: %w", err)
		}
		f.FirstSeen, err = time.Parse(time.RFC3339, firstSeenStr)
		if err != nil {
			slog.Debug("failed to parse first_seen as RFC3339, trying alternative format", "first_seen", firstSeenStr, "error", err)
			f.FirstSeen, err = time.Parse("2006-01-02 15:04:05", firstSeenStr)
			if err != nil {
				slog.Error("failed to parse first_seen timestamp", "first_seen", firstSeenStr, "error", err)
			}
		}
		f.LastSeen, err = time.Parse(time.RFC3339, lastSeenStr)
		if err != nil {
			slog.Debug("failed to parse last_seen as RFC3339, trying alternative format", "last_seen", lastSeenStr, "error", err)
			f.LastSeen, err = time.Parse("2006-01-02 15:04:05", lastSeenStr)
			if err != nil {
				slog.Error("failed to parse last_seen timestamp", "last_seen", lastSeenStr, "error", err)
			}
		}
		failures = append(failures, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate auth failures: %w", err)
	}
	return failures, nil
}
