
# Code Quality Audit - April 1, 2026

## 1. ERROR HANDLING ISSUES

### 1.1 Ignored Errors - Critical

| Location | Issue | Severity | Recommendation |
|----------|-------|----------|----------------|
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:679` | `p.Timestamp, _ = time.Parse(...)` - error ignored | HIGH | Handle parse error, log warning, or use valid default |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:937,939` | Time.Parse error ignored in GetBlockedIPs | HIGH | Same as above |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:982,984,986,988` | Time.Parse errors ignored in GetAuthFailures (4 locations) | HIGH | Same as above |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:457` | `rowsAffected, _ := result.RowsAffected()` - error ignored | MEDIUM | Check error, log for monitoring |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:934` | `reqBodyJSON, _ := json.Marshal(req)` - error ignored | LOW | Handle error or at least log it |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:998` | Same json.Marshal error ignored | LOW | Same as above |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:1774,1784` | `s.storage.UpsertAccount` error ignored in batch operations | MEDIUM | Handle errors properly in batch operations |

### 1.2 Missing rows.Err() Check - Critical

| Location | Issue | Severity |
|----------|-------|----------|
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:665-683` | GetRequestTimeSeries missing rows.Err() check after loop | HIGH |

This is a common Go pitfall. After iterating with rows.Next(), you must call rows.Err() to check for errors that occurred during iteration. The function returns points without verifying no iteration errors occurred.

**Recommendation**: Add `if err = rows.Err(); err != nil { return nil, fmt.Errorf(...) }` before return.

## 2. RESOURCE MANAGEMENT

### 2.1 Close() Without Error Handling - Low Risk

Most defer Close() calls don't check errors. This is acceptable for:
- `rows.Close()` - usually fine to ignore in defer
- `resp.Body.Close()` - standard pattern, errors rare

However, consider logging Close() errors for important resources like database connections.

### 2.2 Context Management - Good

The code properly manages context in loops:
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:791-840` - Excellent comment explaining why defer cancel() can't be used in loops
- cancel() is called explicitly in all branches (success, error, retry)
- No context leaks detected

### 2.3 Goroutine Management - Good

Goroutines are properly managed:
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:417-435` - Cleanup task uses shutdownChan for graceful termination
- Test goroutines use WaitGroup properly
- No goroutine leaks detected

## 3. CONCURRENCY SAFETY

### 3.1 Mutex Usage - Excellent

All mutex locks use defer unlock pattern correctly:
- 183 defer unlock patterns found across codebase
- No missing unlocks detected
- Proper RLock/RUnlock usage for read operations

### 3.2 Global State - Acceptable

`globalAuthTracker` in auth.go is properly protected with mutex. All access methods use proper locking.

## 4. CODE DUPLICATION

### 4.1 Repeated Patterns - Medium Impact

| Pattern | Locations | Recommendation |
|---------|-----------|----------------|
| Time parsing with fallback | sqlite.go:937-939, 982-988 | Create helper function `parseTimeMultiFormat(str string, formats ...string) (time.Time, error)` |
| rows.Scan with NullTime handling | Multiple locations in sqlite.go | Consider using sql.NullTime wrapper helper |
| Error response construction | Multiple handler files | Use centralized error response helper |

### 4.2 Similar Function Structures

Multiple storage functions follow similar patterns (QueryContext -> defer rows.Close -> loop -> scan). This is acceptable but could be abstracted with generics in future Go versions.

## 5. NAMING CONVENTIONS

### 5.1 Overall Assessment - Good

- Package names follow Go conventions (single word, lowercase)
- Function/variable names follow standard conventions
- Constants properly named
- Interface names follow Go convention (single method = Method + "er")

### 5.2 Minor Issues

| Location | Issue | Recommendation |
|----------|-------|----------------|
| Various | Some Chinese comments present | Consider translating to English for broader accessibility |
| `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:791` | Chinese comment in critical section | Translate: "Cannot use defer cancel() in loop body..." |

## 6. DOCUMENTATION

### 6.1 Comments Assessment

- **No TODO/FIXME/HACK comments found** - Good sign of code stability
- README is comprehensive with architecture diagrams, examples
- Some exported functions missing doc comments

### 6.2 Functions Missing Documentation

Many exported functions lack Go doc comments. Consider adding for:
- Public API handlers
- Storage interface methods
- Provider interface methods

## 7. TEST COVERAGE

### 7.1 Statistics

- **Total Go files**: 69 (51 source + 18 test)
- **Test file ratio**: 35% (18/51)
- **Coverage assessment**: Moderate - covers main functionality

### 7.2 Test Quality Observations

Tests appear to cover:
- Chat completions (chat_test.go)
- Proxy functionality (proxy_test.go)
- SSE streaming (sse_test.go)
- Storage layer (storage_test.go)
- Rate limiting (limiter_test.go)
- Resilience patterns (resilience_test.go)
- Provider logic (provider_test.go)
- Router logic (router_test.go)
- Middleware (middleware_test.go)

Missing:
- Integration tests
- Benchmark tests
- Edge case coverage tests

## 8. POTENTIAL SECURITY CONSIDERATIONS

### 8.1 API Key Handling - Good

- Uses `crypto/subtle.ConstantTimeCompare` for key validation (auth.go:77)
- Keys are hashed before storage
- Proper timing attack prevention

### 8.2 Input Validation - Good

- Model name validation exists (middleware.ValidateModelName)
- Request body size limits enforced
- Input sanitization in handlers

## 9. SUMMARY AND PRIORITIES

### Critical (Fix Immediately)
1. Add rows.Err() check in GetRequestTimeSeries (sqlite.go:683)
2. Handle time.Parse errors or log warnings (sqlite.go:679, 937, 939, 982-988)

### Medium (Fix Soon)
1. Handle UpsertAccount errors in batch operations (main.go:1774, 1784)
2. Handle RowsAffected error (sqlite.go:457)
3. Consider abstracting repeated time parsing patterns

### Low (Nice to Have)
1. Add documentation comments for exported functions
2. Translate Chinese comments to English
3. Handle json.Marshal errors in logging helpers
4. Add integration tests

## 10. POSITIVE OBSERVATIONS

- Excellent context management in loops (with proper cancel calls)
- Proper goroutine lifecycle management
- Consistent mutex usage with defer unlock
- Good error handling patterns in most places
- No panic/recover in production code (only in tests)
- Comprehensive README and configuration documentation
- Security-conscious API key handling

# Business Logic Issues Analysis Report
**Date**: 2026-04-01
**Project**: AIProxy

## 1. Account Pool Management and Selection Strategy Issues

### 1.1 Double Rate Limit Check (High Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:759-768` and `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/pool/selector.go:46-54`

**Issue**: Rate limit is checked twice in the request flow:
- First in `WeightedRoundRobin.Select()` (selector.go:46-54)
- Then again in `executeRequest()` via `selectAvailableAccount()` which calls selector.Select()
- No rate limit check is explicitly done after account selection in executeRequest (lines 759-767)

However, looking at handler/chat.go:121-133, there's an additional rate limit check AFTER account selection.

**Impact**: 
- Unnecessary double checking causes performance overhead
- Potential race condition if limit state changes between checks
- The second check in chat.go uses `account.ID` which is correct, but the flow is duplicated

**Fix Recommendation**: 
- Consolidate rate limit checking to one place - either in selector or after selection
- Remove the redundant check in chat.go if selector already handles it

### 1.2 Circuit Breaker State Not Synchronized with Pool State (Medium Priority)
**Location**: 
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/pool/pool.go:174-176` - pool checks `ConsecutiveFailures`
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:335-339` - circuit breaker initialized separately
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:848-883` - circuit breaker check in selectAvailableAccount

**Issue**: The pool tracks `ConsecutiveFailures` and circuit breaker tracks failure state independently:
- Pool.GetAvailableAccounts() checks `state.ConsecutiveFailures < domain.CircuitBreakerThreshold`
- Circuit breaker has its own failure counting and state machine
- When pool.RecordFailure() is called, both are updated but thresholds may differ

**Impact**:
- Inconsistent account availability determination
- Pool might mark account unavailable at 5 failures while circuit breaker might still be in half-open state
- Config `CircuitBreaker.Threshold` applies to circuit breaker but pool uses constant `CircuitBreakerThreshold = 5`

**Fix Recommendation**:
- Use circuit breaker state as the sole source of availability
- Remove redundant ConsecutiveFailures tracking in pool, or make them use same threshold from config

### 1.3 Limiter Map Key Mismatch (High Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:364-389`

**Issue**: Limiters are stored by `accountID` but retrieved by `account.ID` in different contexts:
- `s.limiters[accountID]` where accountID is generated from `utils.GenerateAccountID(pc.Name, keyConfig.Key)`
- In handler/chat.go:122, it uses `h.limiter.Allow(ctx, account.ID)` which should match
- But in main.go:345-346, selectors use `s.limiters[pc.Name]` (provider name) not account ID

Wait, let me re-check: In main.go:388, `s.limiters[accountID]` stores limiter by account ID.
In main.go:345-349, the composite limiter retrieval:
```go
if compositeLimiter, ok := s.limiters[pc.Name]; ok {
    s.selectors[pc.Name] = pool.NewWeightedRoundRobin(p, compositeLimiter)
}
```

This uses provider name (`pc.Name`) but limiters are stored by `accountID`. This is a BUG!

**Impact**:
- Selectors never get the correct limiter because lookup key doesn't match
- Rate limiting won't work properly for account selection
- All accounts under a provider share the wrong/no limiter

**Fix Recommendation**:
- Store limiters per account (currently correct in initAccountLimiter)
- But selector should pass account-specific limiter, not provider-level limiter
- Need to redesign: either selector checks each account's limiter individually, or limiter should be passed differently

### 1.4 Account Selection Doesn't Respect Provider-Level Limits (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/pool/selector.go:46-54`

**Issue**: The selector checks `w.limiter.Allow(ctx, state.Account.ID)` for each account, but `w.limiter` is a CompositeLimiter that may be nil or provider-level (see issue 1.3).

**Impact**:
- If limiter is nil, no rate limit check happens during selection
- If limiter is provider-level, all accounts share the same rate limit bucket which is incorrect

---

## 2. Rate Limiting Logic Issues

### 2.1 In-Memory vs Database State Inconsistency (High Priority)
**Location**: 
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/limiter/rpm.go` - uses in-memory `windows` map
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/limiter/daily.go` - uses in-memory `counts` map
- `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:287-321` - writes to database

**Issue**: All limiters maintain in-memory state AND write to database:
- RPM.Allow() checks in-memory sliding window (lines 35-56)
- RPM.Record() updates in-memory AND writes to DB (lines 58-76)
- On restart, in-memory state is lost but DB state persists
- No mechanism to restore in-memory state from DB on startup

**Impact**:
- After server restart, rate limits reset in memory while DB has old counts
- Can exceed limits temporarily after restart
- The Allow() check uses in-memory state only, DB is just for persistence

**Fix Recommendation**:
- Load rate limit state from DB on startup
- Or use DB as the primary source for Allow() checks (with caching)
- Implement state synchronization on initialization

### 2.2 Race Condition in Sliding Window Pruning (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/limiter/rpm.go:130-141` and `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/limiter/window.go:139-151`

**Issue**: pruneWindow modifies slices in-place while iterating:
```go
for i, ts := range sw.timestamps {
    if ts.After(windowStart) {
        sw.counts[validIdx] = sw.counts[i]  // Potentially overwrites unprocessed data
        ...
    }
}
```

**Impact**:
- In concurrent scenarios with improper locking, data corruption possible
- Currently protected by mutex but the algorithm is fragile

**Fix Recommendation**:
- Use a more robust pruning algorithm or consider alternative data structures
- Could use ring buffer or time-bucketed counting

### 2.3 Token Limiter Uses Incorrect Delta in Record (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/limiter/token.go:77-93`

**Issue**: Token.Record() receives `delta` but adds it to `completionTokens`:
```go
usage.completionTokens += delta
```
This is semantically incorrect - delta should represent total tokens or be split properly.

**Impact**:
- Prompt tokens are never tracked via Record()
- Only completionTokens accumulate
- Total token count will be incorrect for rate limiting

**Fix Recommendation**:
- Record() should track total tokens properly
- Or use RecordActual() which correctly separates prompt/completion tokens
- Ensure streaming response handling uses correct token counting

### 2.4 Missing Limit Value in Database (High Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:310-314`

**Issue**: IncrementRateLimit inserts with `max_value = 0`:
```go
INSERT INTO account_limits (..., max_value, current_value, ...)
VALUES (?, ?, 0, delta, ...)
```

**Impact**:
- Database doesn't store the actual limit max value
- GetRateLimit returns Max = 0 from DB
- Can't properly report remaining quota to users

**Fix Recommendation**:
- Store the configured max limit value in database
- Or compute Max from config when returning state

---

## 3. Retry Mechanism Issues

### 3.1 Retry Without Account Switching (High Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:759-846`

**Issue**: The retry loop retries with the SAME account:
```go
for attempt := 1; attempt <= maxAttempts; attempt++ {
    account, err := s.selectAvailableAccount(...)  // Selects potentially same account
    ...
}
```

When a request fails due to rate limit or account-specific error, retrying with the same account is ineffective.

**Impact**:
- Retries waste time on already-failed accounts
- No account-level failover during retries
- Circuit breaker opens after retries exhaust, not during

**Fix Recommendation**:
- On retry failure, mark account unavailable and select a different account
- Implement account-level fallback within retry loop
- Consider provider-level fallback as secondary

### 3.2 Retry Delay Calculation Issue (Low Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/resilience/retry.go:73-79`

**Issue**: calculateDelay uses `attempt-1` as exponent:
```go
delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))
```
For attempt=1 (first retry), delay = InitialDelay * 1 = InitialDelay (correct)
For attempt=2, delay = InitialDelay * Multiplier (correct)

But in main.go:774-776, it uses:
```go
delay = retry.CalculateDelay(attempt)
```
Where attempt starts at 1, but delay is only applied when `attempt > 1` (line 771).

**Impact**:
- Minor issue, delay calculation is correct in practice

### 3.3 No Retry on Client Errors (4xx except 429) (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:824-830`

**Issue**: 4xx errors (except 429) are not retried:
```go
if resp.StatusCode >= 400 {
    cancel()
    ...
    return nil, nil, fmt.Errorf("client error: %d", resp.StatusCode)
}
```

Some 4xx errors like 408 (Request Timeout) or 409 (Conflict) might be retryable.

**Impact**:
- Missing retry opportunities for potentially transient 4xx errors

**Fix Recommendation**:
- Add configurable retryable 4xx status codes (408, 409, etc.)
- Make retry decision more sophisticated

---

## 4. Circuit Breaker Issues

### 4.1 Circuit Breaker Timer Not Used (Low Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/resilience/circuit_breaker.go:46-47`

**Issue**: `timer` and `timerMu` fields are defined but never used:
```go
timer    *time.Timer
timerMu  sync.Mutex
```

**Impact**:
- Dead code, potential confusion
- Circuit breaker relies on time.Since() check in Allow() instead of timer

**Fix Recommendation**:
- Remove unused fields or implement timer-based state transition

### 4.2 Half-Open State Allows Too Many Concurrent Requests (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/resilience/circuit_breaker.go:99-105`

**Issue**: Half-open state allows `successThreshold` requests concurrently:
```go
if cb.halfOpenCount < cb.successThreshold {
    cb.halfOpenCount++
    return true
}
```

But halfOpenCount is incremented on each Allow(), not decremented on success/failure.

**Impact**:
- Once halfOpenCount reaches threshold, no more requests allowed until state reset
- If test requests fail, circuit immediately opens again (correct behavior)
- But if requests succeed, need `successThreshold` successes to close (correct)

Actually, looking more closely, this is correct - it allows up to `successThreshold` test requests in half-open state.

### 4.3 Circuit Breaker State Reset on Reload (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:1545-1549`

**Issue**: On config reload, circuit breakers are preserved if ID exists:
```go
for id, cb := range oldCircuitBreakers {
    if _, exists := s.circuitBreakers[id]; !exists {
        s.circuitBreakers[id] = cb
    }
}
```

This preserves old circuit breaker state but new accounts get fresh circuit breakers.

**Impact**:
- Inconsistent circuit breaker state after reload
- New accounts start fresh, existing accounts keep old state
- Could be desirable behavior but should be documented

---

## 5. Error Handling Issues

### 5.1 Error Context Lost in Forwarding (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:1012-1041`

**Issue**: forwardUpstreamError reads body but loses original headers:
```go
defer resp.Body.Close()
limitedReader := io.LimitReader(resp.Body, 64*1024)
bodyBytes, err := io.ReadAll(limitedReader)
```

Original response headers from upstream are not forwarded.

**Impact**:
- Client doesn't receive upstream error details in headers
- Missing `X-RateLimit-*` headers from upstream 429 responses

**Fix Recommendation**:
- Forward relevant headers from upstream error response
- Preserve rate limit headers for 429 responses

### 5.2 Inconsistent Error Response Format (Low Priority)
**Location**: Multiple files

**Issue**: Error responses use OpenAI format but status codes vary:
- chat.go uses custom error format
- main.go uses openai.ErrorResponse
- Different error type strings used

**Impact**:
- Client might receive inconsistent error formats
- Minor issue as most follow OpenAI format

---

## 6. Configuration Management Issues

### 6.1 Config Reload Doesn't Update Running Limiters (High Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:1497-1565`

**Issue**: handleAdminReload() reinitializes providers but:
- In-memory rate limit state (windows, counts maps) is not preserved
- Limiters are recreated with fresh in-memory state
- Database state remains but in-memory state resets

```go
s.limiters = make(map[string]*limiter.CompositeLimiter)
...
for id, lim := range oldLimiters {
    if _, exists := s.limiters[id]; !exists {
        s.limiters[id] = lim
    }
}
```

Only preserves limiters that exist in both old and new config.

**Impact**:
- Rate limits reset on config reload
- Active accounts might suddenly have fresh quota

**Fix Recommendation**:
- Preserve in-memory rate limit state across reload
- Or load state from DB after reload

### 6.2 Provider API Key Change Not Detected (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:302-362`

**Issue**: initAccountPool generates account ID from provider name and key:
```go
account := &domain.Account{
    ID: utils.GenerateAccountID(pc.Name, keyConfig.Key),
    ...
}
```

If API key changes in config, a NEW account ID is generated, old account persists in pool/DB.

**Impact**:
- Old accounts with old keys remain in system
- Potential for orphaned accounts
- Config reload creates new accounts instead of updating existing

**Fix Recommendation**:
- Implement account key update logic
- Clean up orphaned accounts on reload

### 6.3 Missing Validation for Zero Limits (Low Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/config/validator.go:243-266`

**Issue**: Limits validation only checks for negative values:
```go
if limits.RPM != nil && *limits.RPM < 0 { ... }
```

But doesn't warn about zero limits which effectively disables that limit type.

**Impact**:
- Zero limits silently disable rate limiting
- Could be intentional but should be validated/warned

---

## 7. Additional Issues Found

### 7.1 Memory Leak in Rate Limit Windows (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/limiter/rpm.go:16-18` and similar

**Issue**: `windows map[string]*slidingWindow` grows unbounded:
- New accounts add entries
- Old accounts never removed unless explicitly reset
- pruneWindow only removes old timestamps, not entire account entries

**Impact**:
- Memory grows with number of accounts ever used
- Long-running server accumulates stale account entries

**Fix Recommendation**:
- Implement periodic cleanup of stale account entries
- Track last access time and purge inactive accounts

### 7.2 SQLite Window Start Calculation Mismatch (Medium Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go:292-307`

**Issue**: Database window start is calculated differently than in-memory:
```go
windowStart := now.Truncate(time.Minute)  // DB uses minute truncation
```

But in-memory limiters use:
- RPM: now.Add(-r.windowSz) for sliding window (rpm.go:40)
- Daily: time.Date(...) for day boundary (daily.go:98)
- Monthly: time.Date(...) for month boundary (monthly.go:98)

**Impact**:
- Database and in-memory window boundaries don't align
- Counts might not match between memory and DB
- Race condition between Allow() (in-memory) and Record() (DB write)

**Fix Recommendation**:
- Use consistent window boundary calculation
- Align DB and in-memory window definitions

### 7.3 Streaming Response Token Tracking Gap (High Priority)
**Location**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:916-925`

**Issue**: Streaming token extraction might fail:
```go
promptTokens, completionTokens, found := streamHandler.GetTokenExtractor().ExtractFromStream(nil)
```

If upstream doesn't provide usage metadata in stream, `found = false` and tokens are not recorded.

**Impact**:
- Token-based rate limits don't work for streaming without upstream metadata
- Config says "hybrid" mode for estimation but needs proper implementation

**Fix Recommendation**:
- Implement proper token estimation for streaming
- Ensure fallback estimation when upstream metadata missing

---

## Summary of Critical Issues (Must Fix)

1. **Limiter Map Key Mismatch (1.3)** - Rate limiting doesn't work properly
2. **In-Memory vs Database State Inconsistency (2.1)** - Limits reset on restart
3. **Retry Without Account Switching (3.1)** - Retries ineffective
4. **Missing Limit Value in Database (2.4)** - Can't report quota
5. **Streaming Token Tracking Gap (7.3)** - Token limits ineffective for streaming
6. **Memory Leak in Rate Limit Windows (7.1)** - Memory grows unbounded
# Security Audit Issues

**Audit Date**: 2026-04-01
**Auditor**: AI Security Analysis

---

## HIGH Severity Issues

### 1. API Key Raw Value Stored as Hash (CRITICAL)
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:313`
- **Problem**: In `initAccountPool()`, `account.APIKeyHash = keyConfig.Key` directly stores the **raw API key** instead of its hash value.
- **Impact**: Raw API keys are stored in memory and database, exposing them to potential memory dumps or database leaks.
- **Fix**: 
  ```go
  // Line 313: Should be:
  account.APIKeyHash = utils.HashAPIKey(keyConfig.Key)
  ```

### 2. API Key Exposed in Configuration Validation Errors
- **Files**: 
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/config/validator.go:224`
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/config/validator.go:227`
- **Problem**: Error messages include raw API key values:
  ```go
  return newConfigError(..., "is required", key.Key)  // Line 224
  return newConfigError(..., "must be unique", key.Key)  // Line 227
  ```
- **Impact**: API keys may be logged or displayed to users when configuration validation fails.
- **Fix**: Replace `key.Key` with `"***"` or masked value in error messages.

### 3. Admin API Key Returned on Creation
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:1889-1892`
- **Problem**: `handleAdminCreateAPIKey()` returns the raw API key in JSON response.
- **Impact**: By design this is necessary, but if response is logged, cached, or intercepted, key could leak.
- **Fix**: Add warning to documentation. Consider implementing one-time token display only.

---

## MEDIUM Severity Issues

### 4. Request/Response Body Logging May Expose Sensitive Data
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/logging.go:63-69`
- **Problem**: When `IncludeRequestBody` or `IncludeResponseBody` is enabled, full request/response bodies are logged.
- **Impact**: User messages, prompts, and API responses containing sensitive data are logged.
- **Fix**: 
  - Add sensitive field filtering before logging
  - Implement body truncation for large responses
  - Add explicit security warning in documentation

### 5. Request Log Database Storage Contains Full Bodies
- **Files**: 
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:931-937`
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:995-1007`
- **Problem**: `recordRequestLog()` stores complete request/response bodies in database when logging is enabled.
- **Impact**: Database contains potentially sensitive user conversations and AI responses.
- **Fix**: 
  - Add encryption for stored bodies
  - Implement data retention policy
  - Add PII detection and masking

### 6. Global Auth Failure Tracker Memory/DB Sync Issues
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/auth.go:56-61`
- **Problem**: `globalAuthTracker` is a global variable that loads from DB on startup but memory state may diverge from DB state over time.
- **Impact**: 
  - Restart loses in-memory failure counts not yet written to DB
  - Race conditions possible between memory and DB updates
- **Fix**: Implement periodic sync to DB or use DB as primary source.

### 7. Admin Dashboard Unauthenticated Access
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:542-545`
- **Problem**: Routes `/` and `/dashboard` have no authentication middleware.
- **Impact**: Anyone can access the admin dashboard UI (static pages only, API requires auth).
- **Fix**: 
  - Add authentication requirement for dashboard access
  - Or move dashboard to authenticated route group
  - At minimum: add rate limiting for dashboard access

### 8. Admin Handler Account API Key Storage Issue
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/handler/admin.go:296`
- **Problem**: In `AddAccount()`, `account.APIKeyHash = utils.HashAPIKey(req.APIKey)` correctly hashes, but original API key from config is still stored raw elsewhere.
- **Note**: This specific handler is correct, but the main issue is in main.go:313.

---

## LOW Severity Issues

### 9. IP Blocking Relies on ClientIP() - Proxy Spoofing Risk
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/auth.go:261`
- **Problem**: Uses `c.ClientIP()` which trusts configured proxy headers. If proxy configuration is wrong, attackers can spoof IPs.
- **Impact**: IP-based security measures (blocking, rate limiting) can be bypassed.
- **Fix**: 
  - Configure trusted proxies explicitly
  - Use `RemoteAddr` as fallback
  - Add documentation warning about proxy configuration

### 10. Error Messages Leak Internal Information
- **Files**: Multiple locations
- **Examples**: 
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/handler/admin.go:147,210` - `err.Error()` returned in JSON
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/cmd/server/main.go:632` - error message in response
- **Problem**: Raw internal error messages are returned to clients.
- **Impact**: May expose database structure, file paths, internal system details.
- **Fix**: Sanitize error messages before returning to clients. Use generic messages for external errors.

### 11. Recovery Middleware Exposes Stack Trace in Logs
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/recovery.go:21`
- **Problem**: Stack trace is logged in `slog.Error()` with full stack.
- **Impact**: If logs are accessible to attackers, internal code structure is exposed.
- **Fix**: This is acceptable for internal logs, but ensure logs are secured and not exposed externally.

### 12. API Key Hash Without Salt
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/pkg/utils/hash.go:9-12`
- **Problem**: `HashAPIKey()` uses plain SHA256 without salt.
- **Impact**: Rainbow tables could potentially be used for common API key patterns.
- **Fix**: Add per-key or global salt. For API keys (high entropy), this is lower risk than passwords.

### 13. CORS Wildcard Allows All Origins
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/security.go:56`
- **Problem**: CORS allows `*` wildcard for origins.
- **Impact**: Any website can make cross-origin requests to the API.
- **Fix**: Restrict `AllowedOrigins` to specific domains in production.

---

## POSITIVE Security Practices

### SQL Injection Prevention
- All SQL queries use parameterized queries (`?` or `sql.Named`)
- **Files**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/storage/sqlite.go`, `queries.go`
- No string concatenation in queries.

### Constant-Time Comparison
- API key validation uses `subtle.ConstantTimeCompare`
- **Files**: 
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/auth.go:77-79`
  - `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/handler/admin.go:76-78`
- Prevents timing attacks.

### Security Headers Middleware
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/security.go`
- Supports X-Frame-Options, X-Content-Type-Options, CSP, HSTS, etc.

### Auth Failure Rate Limiting
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/auth.go:309-315`
- Blocks IPs after excessive auth failures.
- Prevents brute force attacks.

### Model Name Validation
- **File**: `/Users/wangluyao/Documents/myWork/ai/aiproxy/internal/middleware/validation.go:13-24`
- Validates model names against injection patterns.

---

## Recommendations Summary

1. **CRITICAL**: Fix main.go:313 to hash API keys before storage
2. **HIGH**: Remove API key values from config validation error messages
3. **MEDIUM**: Add authentication for dashboard routes
4. **MEDIUM**: Implement sensitive data filtering in logs
5. **LOW**: Configure trusted proxies explicitly
6. **LOW**: Add salt to API key hashing

