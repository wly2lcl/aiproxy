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
