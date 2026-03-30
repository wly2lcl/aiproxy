# AIProxy

A high-performance multi-account LLM API gateway with intelligent load balancing, rate limiting, and resilience features.

[中文文档](README_CN.md)

## Features

### Core Features
- **Multi-Provider Support**: OpenAI, OpenRouter, Groq, and any OpenAI-compatible API
- **Multi-Account Pooling**: Combine multiple API keys for increased throughput
- **Intelligent Load Balancing**: Weighted round-robin with priority support
- **Multi-Dimensional Rate Limiting**: RPM, daily, 5-hour window, monthly, and token limits
- **SQLite Persistence**: Rate limit state survives restarts (WAL mode)
- **Streaming Support**: Full SSE streaming for real-time responses
- **Token Tracking**: Hybrid mode for accurate streaming token counting (estimates tokens if upstream usage metadata is missing)
- **Admin Dashboard**: Built-in web UI for monitoring and management
- **Prometheus Metrics**: Built-in observability
- **Graceful Shutdown**: Zero-downtime restarts

### Resilience Features
- **Retry**: Automatic retry with exponential backoff for transient failures
- **Circuit Breaker**: Per-account circuit breaker to prevent cascading failures
- **Fallback**: Provider-level failover for high availability

## Quick Start

### Using Docker (Recommended)

```bash
# Copy example config
cp config/config.example.json config/config.json

# Edit config with your API keys
vim config/config.json

# Run with Docker Compose
docker compose -f docker/docker-compose.yml up -d

# Check logs
docker compose -f docker/docker-compose.yml logs -f
```

### Building from Source

```bash
# Clone repository
git clone https://github.com/wangluyao/aiproxy.git
cd aiproxy

# Install dependencies
make deps

# Initialize
make init

# Build and run
make run

# Or run directly
go run ./cmd/server
```

## Configuration

See [config/config.example.json](config/config.example.json) for a complete configuration example.

### Minimal Configuration

```json
{
  "server": {
    "port": 8080
  },
  "providers": [
    {
      "name": "openrouter",
      "api_base": "https://openrouter.ai/api/v1",
      "api_keys": [
        {"key": "sk-or-xxx", "weight": 1, "limits": {"rpm": 20, "daily": 100}}
      ]
    }
  ]
}
```

### Configuration Fields

| Field | Description | Default |
|-------|-------------|---------|
| `server.port` | API server port (public + admin merged) | 8080 |
| `server.host` | API server host | 0.0.0.0 |
| `database.path` | SQLite database path | data/aiproxy.db |
| `database.max_open_conns` | SQLite max open connections | 25 |
| `database.max_idle_conns` | SQLite max idle connections | 25 |
| `auth.enabled` | Enable API key authentication | false |
| `auth.api_keys` | List of valid API keys | [] |
| `admin.enabled` | Enable admin API and dashboard | true |
| `admin.api_keys` | Admin API authentication keys (required for security) | [] |

### Provider Configuration

```json
{
  "name": "openrouter",
  "api_base": "https://openrouter.ai/api/v1",
  "models": ["openai/gpt-4o-mini", "anthropic/claude-3-haiku"],
  "api_keys": [
    {
      "key": "sk-or-xxx",
      "name": "account-1",
      "weight": 2,
      "priority": 2,
      "limits": {
        "rpm": 20,
        "daily": 100,
        "window_5h": 50,
        "monthly": 3000,
        "token_daily": 100000,
        "token_monthly": 3000000
      }
    }
  ],
  "retry": {
    "max_retries": 3,
    "initial_wait": "1s",
    "max_wait": "30s",
    "multiplier": 2.0
  },
  "circuit_breaker": {
    "threshold": 5,
    "timeout": "60s"
  }
}
```

#### Account Selection

- **priority**: Higher value = higher priority. Accounts with higher priority are used first.
- **weight**: Within the same priority group, requests are distributed proportionally by weight.
```

### Environment Variables

Override configuration with environment variables:

```bash
export AIPROXY_SERVER_PORT=9090
export AIPROXY_DATABASE_PATH=/data/aiproxy.db
export AIPROXY_LOGGING_LEVEL=debug
```

## API Endpoints

All endpoints are served on port 8080.

### Public API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/chat/completions` | POST | Chat completions (OpenAI compatible) |
| `/v1/models` | GET | List available models |
| `/health` | GET | Health check |
| `/ready` | GET | Readiness check |
| `/metrics` | GET | Prometheus metrics |

### Admin API & Dashboard

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/` | GET | None | Admin dashboard |
| `/dashboard` | GET | None | Admin dashboard |
| `/admin/accounts` | GET | API Key | List all accounts |
| `/admin/accounts/:id` | GET | API Key | Get account details |
| `/admin/accounts` | POST | API Key | Add new account |
| `/admin/accounts/:id` | PUT | API Key | Update account |
| `/admin/accounts/:id` | DELETE | API Key | Delete account |
| `/admin/accounts/:id/reset` | POST | API Key | Reset rate limits |
| `/admin/api-keys` | GET | API Key | List API keys |
| `/admin/api-keys` | POST | API Key | Create API key |
| `/admin/stats` | GET | API Key | JSON statistics |
| `/admin/stats/timeseries` | GET | API Key | Time series data |
| `/admin/providers` | GET | API Key | List providers |
| `/admin/logs` | GET | API Key | Recent request logs |
| `/admin/reload` | POST | API Key | Reload configuration |
| `/admin/export/:type` | GET | API Key | Export data (json/csv) |

> **Security Note**: Admin API endpoints require authentication. Configure `admin.api_keys` to protect these endpoints.

## Usage Examples

### Chat Completion

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-api-key" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Streaming

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-api-key" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### List Models

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer sk-your-api-key"
```

### Admin: Get Account Stats

```bash
curl http://localhost:8080/admin/accounts \
  -H "Authorization: Bearer your-admin-key"
```

### Admin: Reload Configuration

```bash
curl -X POST http://localhost:8080/admin/reload \
  -H "Authorization: Bearer your-admin-key"
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     AIProxy (Port 8080)                  │
├─────────────────────────────────────────────────────────┤
│  Public API                                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │  Proxy   │ │  Router  │ │  Models  │                │
│  └────┬─────┘ └────┬─────┘ └──────────┘                │
│       │            │                                    │
│  ┌────▼────┐  ┌────▼────┐                              │
│  │  Pool   │  │ Limiter │                              │
│  └────┬────┘  └────┬────┘                              │
│       │            │                                    │
│  ┌────▼────────────▼────┐                              │
│  │   SQLite Storage     │                              │
│  └──────────────────────┘                              │
├─────────────────────────────────────────────────────────┤
│  Admin Dashboard & API (/admin/*)                       │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │ Accounts │ │  Stats   │ │  Config  │                │
│  └──────────┘ └──────────┘ └──────────┘                │
└─────────────────────────────────────────────────────────┘
```

### Request Flow

1. Request arrives at `/v1/chat/completions`
2. Auth middleware validates API key
3. Router resolves provider based on model name
4. Pool selects best available account (weighted round-robin)
5. Limiter checks rate limits
6. Proxy forwards request to upstream
7. Response streamed back to client
8. Usage stats recorded

## Rate Limiting

### Supported Limit Types

| Type | Description | Window |
|------|-------------|--------|
| `rpm` | Requests per minute | Sliding 60s |
| `daily` | Requests per day | UTC midnight |
| `window_5h` | Requests per 5 hours | Rolling |
| `monthly` | Requests per month | UTC 1st |
| `token_daily` | Tokens per day | UTC midnight |
| `token_monthly` | Tokens per month | UTC 1st |

## Resilience

### Retry

Automatic retry for transient failures with exponential backoff.

```json
"retry": {
  "max_retries": 3,
  "initial_wait": "1s",
  "max_wait": "30s",
  "multiplier": 2.0
}
```

Triggers on: HTTP 429, 500, 502, 503, 504

### Circuit Breaker

Per-account circuit breaker to prevent cascading failures.

```json
"circuit_breaker": {
  "threshold": 5,
  "timeout": "60s"
}
```

- **Closed**: Normal operation
- **Open**: Failing fast, skip this account
- **Half-Open**: Testing recovery

### Fallback

Provider-level failover for high availability.

```json
"fallback": {
  "enabled": true,
  "strategy": "sequential",
  "providers": ["openrouter", "openai", "groq"]
}
```

Flow: `openrouter fails → openai fails → groq → return result`

## Development

### Prerequisites

- Go 1.26+
- Make (optional)

### Make Commands

```bash
make build        # Build optimized binary (~23MB)
make run          # Build and run
make test         # Run all tests
make test-coverage # Run tests with coverage
make lint         # Run linter
make docker       # Build Docker image (~25MB)
make clean        # Clean build artifacts
```

### Build

```bash
make build
```

### Build Options

```bash
# Standard build (~35MB)
make build

# Optimized build (~23MB, recommended for production)
make build-min

# Manual optimized build
CGO_ENABLED=0 go build -ldflags="-s -w -buildid=" -trimpath -o server ./cmd/server
```

### Project Structure

```
aiproxy/
├── cmd/server/main.go       # Entry point
├── internal/
│   ├── config/              # Configuration loading
│   ├── domain/              # Domain types
│   ├── handler/             # HTTP handlers
│   ├── limiter/             # Rate limiting
│   ├── middleware/          # HTTP middleware
│   ├── pool/                # Account pool & selector
│   ├── provider/            # Provider adapters
│   ├── proxy/               # Reverse proxy
│   ├── resilience/          # Retry & circuit breaker
│   ├── router/              # Model routing
│   ├── stats/               # Metrics collection
│   └── storage/             # SQLite storage
├── pkg/
│   ├── openai/              # OpenAI API types
│   └── utils/               # Utilities
├── config/
│   └── config.example.json  # Example configuration
├── migrations/              # Database migrations
├── docker/
│   ├── Dockerfile           # Docker image definition
│   ├── Dockerfile.goreleaser # GoReleaser Docker image
│   ├── docker-compose.yml   # Docker Compose configuration
│   └── entrypoint.sh        # Docker entrypoint script
```

## Monitoring

### Prometheus Metrics

Available at `/metrics`:

```
aiproxy_requests_total{provider, model, status}
aiproxy_request_duration_seconds{provider, model}
aiproxy_tokens_total{provider, model, type}
aiproxy_errors_total{provider, model, error_type}
aiproxy_ratelimit_hits_total{account_id, limit_type}
```

### Grafana Dashboard

Import the provided dashboard for visualization.

## License

MIT License