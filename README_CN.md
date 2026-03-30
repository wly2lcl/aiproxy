# AIProxy

一个高性能的多账号 LLM API 网关，支持智能负载均衡、多维度限流和弹性机制。

[English](README.md)

## 功能特性

### 核心功能
- **多平台支持**：OpenAI、OpenRouter、Groq 以及任何兼容 OpenAI API 的服务
- **多账号池**：合并多个 API Key 以提升吞吐量
- **智能负载均衡**：加权轮询算法，支持优先级调度
- **多维度限流**：RPM（每分钟）、每日、5小时窗口、每月、Token 限制
- **SQLite 持久化**：限流状态跨重启保持（WAL 模式）
- **流式响应**：完整的 SSE 流式传输支持
- **Token 追踪**：混合模式精确统计流式响应的 Token 用量
- **管理后台**：内置 Web UI 监控管理界面
- **Prometheus 指标**：内置可观测性支持
- **优雅关闭**：零停机重启

### 弹性机制
- **自动重试**：指数退避重试，应对临时故障
- **熔断器**：账户级熔断保护，防止级联故障
- **故障转移**：Provider 级别故障转移，保障高可用

## 快速开始

### 使用 Docker（推荐）

```bash
# 复制示例配置
cp config/config.example.json config/config.json

# 编辑配置，填入你的 API Key
vim config/config.json

# 使用 Docker Compose 启动
docker compose -f docker/docker-compose.yml up -d

# 查看日志
docker compose -f docker/docker-compose.yml logs -f
```

### 从源码构建

```bash
# 克隆仓库
git clone https://github.com/wangluyao/aiproxy.git
cd aiproxy

# 安装依赖
make deps

# 初始化
make init

# 构建并运行
make run

# 或直接运行
go run ./cmd/server
```

## 配置说明

完整配置示例请参考 [config/config.example.json](config/config.example.json)。

### 最简配置

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

### 配置字段说明

| 字段 | 描述 | 默认值 |
|-----|------|-------|
| `server.port` | API 服务器端口（公共 + 管理合并） | 8080 |
| `server.host` | API 服务器主机 | 0.0.0.0 |
| `database.path` | SQLite 数据库路径 | data/aiproxy.db |
| `auth.enabled` | 启用 API Key 认证 | false |
| `auth.api_keys` | 有效 API Key 列表 | [] |
| `admin.enabled` | 启用管理 API 和后台 | true |
| `admin.api_keys` | 管理 API 认证密钥（安全必需） | [] |

### Provider 配置

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

#### 账号选择

- **priority**：值越大优先级越高，优先使用高优先级账户
- **weight**：同一优先级组内，按权重比例分配请求
```

### 环境变量

使用环境变量覆盖配置：

```bash
export AIPROXY_SERVER_PORT=9090
export AIPROXY_DATABASE_PATH=/data/aiproxy.db
export AIPROXY_LOGGING_LEVEL=debug
```

## API 端点

所有端点均在 8080 端口提供服务。

### 公开 API

| 端点 | 方法 | 描述 |
|-----|------|------|
| `/v1/chat/completions` | POST | Chat 补全（兼容 OpenAI） |
| `/v1/models` | GET | 列出可用模型 |
| `/health` | GET | 健康检查 |
| `/ready` | GET | 就绪检查 |
| `/metrics` | GET | Prometheus 指标 |

### 管理 API 与后台

| 端点 | 方法 | 认证 | 描述 |
|-----|------|------|------|
| `/` | GET | 无 | 管理后台首页 |
| `/dashboard` | GET | 无 | 管理后台 |
| `/admin/accounts` | GET | API Key | 列出所有账号 |
| `/admin/accounts/:id` | GET | API Key | 获取账号详情 |
| `/admin/accounts` | POST | API Key | 添加新账号 |
| `/admin/accounts/:id` | PUT | API Key | 更新账号 |
| `/admin/accounts/:id` | DELETE | API Key | 删除账号 |
| `/admin/accounts/:id/reset` | POST | API Key | 重置限流计数 |
| `/admin/api-keys` | GET | API Key | 列出 API Key |
| `/admin/api-keys` | POST | API Key | 创建 API Key |
| `/admin/stats` | GET | API Key | JSON 统计数据 |
| `/admin/stats/timeseries` | GET | API Key | 时序数据 |
| `/admin/providers` | GET | API Key | 列出 Provider |
| `/admin/logs` | GET | API Key | 最近请求日志 |
| `/admin/reload` | POST | API Key | 重新加载配置 |
| `/admin/export/:type` | GET | API Key | 导出数据 (json/csv) |

> **安全提示**：管理 API 端点需要认证。请配置 `admin.api_keys` 以保护这些端点。

## 使用示例

### Chat 补全

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-api-key" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "你好！"}]
  }'
```

### 流式响应

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-api-key" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "你好！"}],
    "stream": true
  }'
```

### 列出模型

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer sk-your-api-key"
```

### 管理：获取账号统计

```bash
curl http://localhost:8080/admin/accounts \
  -H "Authorization: Bearer your-admin-key"
```

### 管理：重载配置

```bash
curl -X POST http://localhost:8080/admin/reload \
  -H "Authorization: Bearer your-admin-key"
```

## 架构图

```
┌─────────────────────────────────────────────────────────┐
│                  AIProxy (端口 8080)                     │
├─────────────────────────────────────────────────────────┤
│  公开 API                                                │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │  代理    │ │  路由    │ │  模型    │                │
│  └────┬─────┘ └────┬─────┘ └──────────┘                │
│       │            │                                    │
│  ┌────▼────┐  ┌────▼────┐                              │
│  │ 账号池  │ │  限流器  │                              │
│  └────┬────┘  └────┬────┘                              │
│       │            │                                    │
│  ┌────▼────────────▼────┐                              │
│  │   SQLite 存储        │                              │
│  └──────────────────────┘                              │
├─────────────────────────────────────────────────────────┤
│  管理后台 & API (/admin/*)                               │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │  账号    │ │  统计    │ │  配置    │                │
│  └──────────┘ └──────────┘ └──────────┘                │
└─────────────────────────────────────────────────────────┘
```

### 请求流程

1. 请求到达 `/v1/chat/completions`
2. 认证中间件验证 API Key
3. 路由器根据模型名解析 Provider
4. 账号池选择最佳可用账号（加权轮询）
5. 限流器检查限流状态
6. 代理转发请求到上游
7. 响应流式返回给客户端
8. 记录使用统计

## 限流说明

### 支持的限流类型

| 类型 | 描述 | 窗口 |
|-----|------|------|
| `rpm` | 每分钟请求数 | 滑动 60 秒 |
| `daily` | 每日请求数 | UTC 午夜重置 |
| `window_5h` | 每 5 小时请求数 | 滚动窗口 |
| `monthly` | 每月请求数 | UTC 每月 1 日重置 |
| `token_daily` | 每日 Token 数 | UTC 午夜重置 |
| `token_monthly` | 每月 Token 数 | UTC 每月 1 日重置 |

## 弹性机制

### 自动重试

临时故障自动重试，支持指数退避。

```json
"retry": {
  "max_retries": 3,
  "initial_wait": "1s",
  "max_wait": "30s",
  "multiplier": 2.0
}
```

触发条件：HTTP 429, 500, 502, 503, 504

### 熔断器

账户级熔断保护，防止级联故障。

```json
"circuit_breaker": {
  "threshold": 5,
  "timeout": "60s"
}
```

- **关闭（Closed）**：正常运行
- **打开（Open）**：快速失败，跳过该账户
- **半开（Half-Open）**：恢复探测

### 故障转移

Provider 级别故障转移，保障高可用。

```json
"fallback": {
  "enabled": true,
  "strategy": "sequential",
  "providers": ["openrouter", "openai", "groq"]
}
```

流程：`openrouter 失败 → openai 失败 → groq → 返回结果`

## 开发指南

### 环境要求

- Go 1.26+
- Make（可选）

### Make 命令

```bash
make build        # 构建优化二进制（约 23MB）
make run          # 构建并运行
make test         # 运行测试
make test-coverage # 运行测试并生成覆盖率报告
make lint         # 运行代码检查
make docker       # 构建 Docker 镜像（约 25MB）
make clean        # 清理构建产物
```

### 构建

```bash
make build
```

### 项目结构

```
aiproxy/
├── cmd/server/main.go       # 入口
├── internal/
│   ├── config/              # 配置加载
│   ├── domain/              # 领域类型
│   ├── handler/             # HTTP 处理器
│   ├── limiter/             # 限流器
│   ├── middleware/          # HTTP 中间件
│   ├── pool/                # 账号池与选择器
│   ├── provider/            # Provider 适配器
│   ├── proxy/               # 反向代理
│   ├── resilience/          # 重试与熔断
│   ├── router/              # 模型路由
│   ├── stats/               # 指标收集
│   └── storage/             # SQLite 存储
├── pkg/
│   ├── openai/              # OpenAI API 类型
│   └── utils/               # 工具函数
├── config/
│   └── config.example.json  # 示例配置
├── migrations/              # 数据库迁移
├── docker/
│   ├── Dockerfile           # Docker 镜像定义
│   ├── Dockerfile.goreleaser # GoReleaser Docker 镜像
│   ├── docker-compose.yml   # Docker Compose 配置
│   └── entrypoint.sh        # Docker 入口脚本
```

## 监控

### Prometheus 指标

在 `/metrics` 端点可用：

```
aiproxy_requests_total{provider, model, status}
aiproxy_request_duration_seconds{provider, model}
aiproxy_tokens_total{provider, model, type}
aiproxy_errors_total{provider, model, error_type}
aiproxy_ratelimit_hits_total{account_id, limit_type}
```

### 使用场景示例

#### 场景 1：OpenRouter 多账号

OpenRouter 免费版限制每分钟 20 次请求，每日 50 次。通过配置多个账号：

```json
{
  "providers": [{
    "name": "openrouter",
    "api_base": "https://openrouter.ai/api/v1",
    "api_keys": [
      {"key": "sk-or-account1", "weight": 1, "limits": {"rpm": 20, "daily": 50}},
      {"key": "sk-or-account2", "weight": 1, "limits": {"rpm": 20, "daily": 50}},
      {"key": "sk-or-account3", "weight": 1, "limits": {"rpm": 20, "daily": 50}}
    ]
  }]
}
```

效果：
- RPM 提升到 60 次/分钟
- 每日请求提升到 150 次

#### 场景 2：多平台混合使用

```json
{
  "providers": [
    {
      "name": "openrouter",
      "api_base": "https://openrouter.ai/api/v1",
      "api_keys": [{"key": "sk-or-xxx", "weight": 1}]
    },
    {
      "name": "groq",
      "api_base": "https://api.groq.com/openai/v1",
      "api_keys": [{"key": "gsk_xxx", "weight": 2}]
    }
  ]
}
```

## 许可证

MIT License