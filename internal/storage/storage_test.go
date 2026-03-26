package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
)

func TestSQLite_UpsertProvider(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsDefault: true,
		IsEnabled: true,
	}

	err := db.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider failed: %v", err)
	}

	retrieved, err := db.GetProvider(ctx, "test-provider")
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	if retrieved.ID != provider.ID {
		t.Errorf("expected ID %s, got %s", provider.ID, retrieved.ID)
	}
	if retrieved.APIBase != provider.APIBase {
		t.Errorf("expected APIBase %s, got %s", provider.APIBase, retrieved.APIBase)
	}
	if retrieved.IsDefault != provider.IsDefault {
		t.Errorf("expected IsDefault %v, got %v", provider.IsDefault, retrieved.IsDefault)
	}
	if retrieved.IsEnabled != provider.IsEnabled {
		t.Errorf("expected IsEnabled %v, got %v", provider.IsEnabled, retrieved.IsEnabled)
	}

	provider.APIBase = "https://api2.example.com"
	err = db.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider update failed: %v", err)
	}

	retrieved, err = db.GetProvider(ctx, "test-provider")
	if err != nil {
		t.Fatalf("GetProvider after update failed: %v", err)
	}

	if retrieved.APIBase != "https://api2.example.com" {
		t.Errorf("expected updated APIBase, got %s", retrieved.APIBase)
	}
}

func TestSQLite_GetProvider(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	_, err := db.GetProvider(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
	domainErr, ok := err.(*domain.DomainError)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if domainErr.Code != domain.ErrCodeProviderNotFound {
		t.Errorf("expected error code %s, got %s", domain.ErrCodeProviderNotFound, domainErr.Code)
	}
}

func TestSQLite_ListProviders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	providers := []*domain.Provider{
		{ID: "provider1", APIBase: "https://api1.example.com", IsEnabled: true},
		{ID: "provider2", APIBase: "https://api2.example.com", IsEnabled: true},
		{ID: "provider3", APIBase: "https://api3.example.com", IsEnabled: false},
	}

	for _, p := range providers {
		err := db.UpsertProvider(ctx, p)
		if err != nil {
			t.Fatalf("UpsertProvider failed: %v", err)
		}
	}

	list, err := db.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders failed: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("expected 3 providers, got %d", len(list))
	}
}

func TestSQLite_UpsertAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}
	err := db.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider failed: %v", err)
	}

	account := &domain.Account{
		ID:         "test-account",
		ProviderID: "test-provider",
		APIKeyHash: "hash123",
		Weight:     1,
		Priority:   0,
		IsEnabled:  true,
	}

	err = db.UpsertAccount(ctx, account)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}

	retrieved, err := db.GetAccount(ctx, "test-account")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	if retrieved.ID != account.ID {
		t.Errorf("expected ID %s, got %s", account.ID, retrieved.ID)
	}
	if retrieved.ProviderID != account.ProviderID {
		t.Errorf("expected ProviderID %s, got %s", account.ProviderID, retrieved.ProviderID)
	}
	if retrieved.APIKeyHash != account.APIKeyHash {
		t.Errorf("expected APIKeyHash %s, got %s", account.APIKeyHash, retrieved.APIKeyHash)
	}

	account.Weight = 2
	err = db.UpsertAccount(ctx, account)
	if err != nil {
		t.Fatalf("UpsertAccount update failed: %v", err)
	}

	retrieved, err = db.GetAccount(ctx, "test-account")
	if err != nil {
		t.Fatalf("GetAccount after update failed: %v", err)
	}

	if retrieved.Weight != 2 {
		t.Errorf("expected updated Weight 2, got %d", retrieved.Weight)
	}
}

func TestSQLite_GetAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	_, err := db.GetAccount(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent account")
	}
	domainErr, ok := err.(*domain.DomainError)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if domainErr.Code != domain.ErrCodeAccountNotFound {
		t.Errorf("expected error code %s, got %s", domain.ErrCodeAccountNotFound, domainErr.Code)
	}
}

func TestSQLite_ListAccounts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}
	err := db.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider failed: %v", err)
	}

	accounts := []*domain.Account{
		{ID: "account1", ProviderID: "test-provider", APIKeyHash: "hash1", Weight: 1, Priority: 0, IsEnabled: true},
		{ID: "account2", ProviderID: "test-provider", APIKeyHash: "hash2", Weight: 2, Priority: 1, IsEnabled: true},
		{ID: "account3", ProviderID: "test-provider", APIKeyHash: "hash3", Weight: 1, Priority: 0, IsEnabled: false},
	}

	for _, a := range accounts {
		err = db.UpsertAccount(ctx, a)
		if err != nil {
			t.Fatalf("UpsertAccount failed: %v", err)
		}
	}

	list, err := db.ListAccounts(ctx, "test-provider")
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(list))
	}

	if list[0].Priority < list[1].Priority {
		t.Errorf("accounts should be sorted by priority DESC")
	}
}

func TestSQLite_GetRateLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	account := setupTestAccount(t, db, ctx)

	state, err := db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit failed: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for nonexistent rate limit")
	}

	err = db.IncrementRateLimit(ctx, account.ID, domain.LimitTypeRPM, 1)
	if err != nil {
		t.Fatalf("IncrementRateLimit failed: %v", err)
	}

	state, err = db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit failed: %v", err)
	}
	if state == nil {
		t.Fatal("expected rate limit state")
	}
	if state.Current != 1 {
		t.Errorf("expected current 1, got %d", state.Current)
	}
	if state.Type != domain.LimitTypeRPM {
		t.Errorf("expected type %s, got %s", domain.LimitTypeRPM, state.Type)
	}
}

func TestSQLite_IncrementRateLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	account := setupTestAccount(t, db, ctx)

	err := db.IncrementRateLimit(ctx, account.ID, domain.LimitTypeRPM, 5)
	if err != nil {
		t.Fatalf("IncrementRateLimit failed: %v", err)
	}

	state, err := db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit failed: %v", err)
	}
	if state.Current != 5 {
		t.Errorf("expected current 5, got %d", state.Current)
	}

	err = db.IncrementRateLimit(ctx, account.ID, domain.LimitTypeRPM, 3)
	if err != nil {
		t.Fatalf("IncrementRateLimit second call failed: %v", err)
	}

	state, err = db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit failed: %v", err)
	}
	if state.Current != 8 {
		t.Errorf("expected current 8, got %d", state.Current)
	}
}

func TestSQLite_ResetRateLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	account := setupTestAccount(t, db, ctx)

	err := db.IncrementRateLimit(ctx, account.ID, domain.LimitTypeRPM, 5)
	if err != nil {
		t.Fatalf("IncrementRateLimit failed: %v", err)
	}

	state, err := db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit failed: %v", err)
	}
	if state == nil {
		t.Fatal("expected rate limit state")
	}

	err = db.ResetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("ResetRateLimit failed: %v", err)
	}

	state, err = db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit after reset failed: %v", err)
	}
	if state != nil {
		t.Error("expected nil state after reset")
	}
}

func TestSQLite_RecordTokenUsage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	account := setupTestAccount(t, db, ctx)

	usage := &TokenUsage{
		RequestID:        "req-123",
		AccountID:        account.ID,
		ProviderID:       account.ProviderID,
		Model:            "gpt-4",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		IsStreaming:      true,
		IsEstimated:      false,
	}

	err := db.RecordTokenUsage(ctx, usage)
	if err != nil {
		t.Fatalf("RecordTokenUsage failed: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	summary, err := db.GetTokenUsage(ctx, account.ID, since)
	if err != nil {
		t.Fatalf("GetTokenUsage failed: %v", err)
	}

	if summary.RequestCount != 1 {
		t.Errorf("expected request count 1, got %d", summary.RequestCount)
	}
	if summary.TotalPromptTokens != 100 {
		t.Errorf("expected total prompt tokens 100, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCompletionTokens != 50 {
		t.Errorf("expected total completion tokens 50, got %d", summary.TotalCompletionTokens)
	}
	if summary.TotalTokens != 150 {
		t.Errorf("expected total tokens 150, got %d", summary.TotalTokens)
	}
	if summary.StreamingCount != 1 {
		t.Errorf("expected streaming count 1, got %d", summary.StreamingCount)
	}
}

func TestSQLite_GetTokenUsage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	account := setupTestAccount(t, db, ctx)

	since := time.Now().Add(-1 * time.Hour)
	summary, err := db.GetTokenUsage(ctx, account.ID, since)
	if err != nil {
		t.Fatalf("GetTokenUsage failed: %v", err)
	}

	if summary.RequestCount != 0 {
		t.Errorf("expected request count 0, got %d", summary.RequestCount)
	}

	usages := []*TokenUsage{
		{
			RequestID:        "req-1",
			AccountID:        account.ID,
			ProviderID:       account.ProviderID,
			Model:            "gpt-4",
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			IsStreaming:      true,
			IsEstimated:      false,
		},
		{
			RequestID:        "req-2",
			AccountID:        account.ID,
			ProviderID:       account.ProviderID,
			Model:            "gpt-3.5-turbo",
			PromptTokens:     200,
			CompletionTokens: 100,
			TotalTokens:      300,
			IsStreaming:      false,
			IsEstimated:      true,
		},
	}

	for _, u := range usages {
		err = db.RecordTokenUsage(ctx, u)
		if err != nil {
			t.Fatalf("RecordTokenUsage failed: %v", err)
		}
	}

	summary, err = db.GetTokenUsage(ctx, account.ID, since)
	if err != nil {
		t.Fatalf("GetTokenUsage failed: %v", err)
	}

	if summary.RequestCount != 2 {
		t.Errorf("expected request count 2, got %d", summary.RequestCount)
	}
	if summary.TotalPromptTokens != 300 {
		t.Errorf("expected total prompt tokens 300, got %d", summary.TotalPromptTokens)
	}
	if summary.TotalCompletionTokens != 150 {
		t.Errorf("expected total completion tokens 150, got %d", summary.TotalCompletionTokens)
	}
	if summary.TotalTokens != 450 {
		t.Errorf("expected total tokens 450, got %d", summary.TotalTokens)
	}
	if summary.StreamingCount != 1 {
		t.Errorf("expected streaming count 1, got %d", summary.StreamingCount)
	}
	if summary.EstimatedCount != 1 {
		t.Errorf("expected estimated count 1, got %d", summary.EstimatedCount)
	}

	future := time.Now().Add(1 * time.Hour)
	summary, err = db.GetTokenUsage(ctx, account.ID, future)
	if err != nil {
		t.Fatalf("GetTokenUsage with future time failed: %v", err)
	}

	if summary.RequestCount != 0 {
		t.Errorf("expected request count 0 for future time, got %d", summary.RequestCount)
	}
}

func TestSQLite_Transaction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}

	for i := 0; i < 10; i++ {
		err := db.UpsertProvider(ctx, provider)
		if err != nil {
			t.Fatalf("UpsertProvider iteration %d failed: %v", i, err)
		}
	}

	retrieved, err := db.GetProvider(ctx, "test-provider")
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected provider to exist")
	}
}

func TestSQLite_ConcurrentWrites(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}
	err := db.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider failed: %v", err)
	}

	account := &domain.Account{
		ID:         "test-account",
		ProviderID: "test-provider",
		APIKeyHash: "hash123",
		Weight:     1,
		Priority:   0,
		IsEnabled:  true,
	}
	err = db.UpsertAccount(ctx, account)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}

	const numGoroutines = 10
	const numIncrements = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numIncrements; j++ {
				err := db.IncrementRateLimit(ctx, account.ID, domain.LimitTypeRPM, 1)
				if err != nil {
					t.Errorf("IncrementRateLimit failed: %v", err)
				}
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	state, err := db.GetRateLimit(ctx, account.ID, domain.LimitTypeRPM)
	if err != nil {
		t.Fatalf("GetRateLimit failed: %v", err)
	}

	expected := numGoroutines * numIncrements
	if state.Current != expected {
		t.Errorf("expected current %d, got %d", expected, state.Current)
	}
}

func TestSQLite_Migrations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db1, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}
	db1.Close()

	db2, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite on existing database failed: %v", err)
	}
	defer db2.Close()

	ctx := context.Background()
	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}
	err = db2.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider after migrations failed: %v", err)
	}
}

func setupTestDB(t *testing.T) *SQLite {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}

	return db
}

func setupTestAccount(t *testing.T, db *SQLite, ctx context.Context) *domain.Account {
	t.Helper()

	provider := &domain.Provider{
		ID:        "test-provider",
		APIBase:   "https://api.example.com",
		IsEnabled: true,
	}
	err := db.UpsertProvider(ctx, provider)
	if err != nil {
		t.Fatalf("UpsertProvider failed: %v", err)
	}

	account := &domain.Account{
		ID:         "test-account",
		ProviderID: "test-provider",
		APIKeyHash: "hash123",
		Weight:     1,
		Priority:   0,
		IsEnabled:  true,
	}
	err = db.UpsertAccount(ctx, account)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}

	return account
}
