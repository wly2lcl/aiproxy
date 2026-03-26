# AIProxy 配置说明

## 配置文件格式

- `config.example.json` - 标准JSON格式，可直接复制使用
- `config.example.jsonc` - 带详细中文注释的版本，便于理解配置项

## 快速开始

```bash
# 复制配置模板
cp config/config.example.json config/config.json

# 编辑配置，填入你的 API Key
vim config/config.json
```

---

## 配置项详解

### 服务器配置 (`server`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `host` | string | `0.0.0.0` | 监听地址，`0.0.0.0` 接受所有网卡请求 |
| `port` | int | `8080` | 公共API端口 |
| `read_timeout` | duration | `30s` | 读取请求超时时间 |
| `write_timeout` | duration | `120s` | 写入响应超时时间（非流式） |
| `idle_timeout` | duration | `120s` | Keep-Alive 连接空闲超时 |
| `graceful_shutdown_timeout` | duration | `30s` | 优雅关闭等待时间 |
| `max_request_body_size` | int | `10485760` | 请求体最大大小（字节） |

### 数据库配置 (`database`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `path` | string | `data/aiproxy.db` | SQLite 数据库文件路径 |
| `busy_timeout` | int | `5000` | 数据库繁忙等待超时（毫秒） |
| `journal_mode` | string | `WAL` | 日志模式，WAL 提供更好并发性能 |
| `cache_size` | int | `-64000` | 缓存大小（KB，负数） |
| `auto_vacuum` | string | `INCREMENTAL` | 自动清理模式 |

### 日志配置 (`logging`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `level` | string | `info` | 日志级别：`debug`, `info`, `warn`, `error` |
| `format` | string | `json` | 日志格式：`json`, `text` |
| `output` | string | `stdout` | 输出位置：`stdout`, `stderr`, 或文件路径 |
| `include_request_body` | bool | `false` | 是否记录请求体 |
| `include_response_body` | bool | `false` | 是否记录响应体 |

### 认证配置 (`auth`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用 API Key 认证 |
| `api_keys` | []string | `[]` | 有效的 API Key 列表 |
| `header_name` | string | `Authorization` | 认证头名称 |
| `key_prefix` | string | `Bearer ` | API Key 前缀 |

---

### Provider 配置 (`providers`)

每个 Provider 的配置结构：

```json
{
  "name": "openrouter",           // Provider 标识符
  "api_base": "https://...",      // API 基础地址
  "models": ["model-1", ...],     // 支持的模型列表
  "is_default": true,             // 是否为默认 Provider
  "is_enabled": true,             // 是否启用
  "headers": {},                  // 自定义请求头
  "timeout": "30s",               // 非流式超时
  "stream_timeout": "120s",       // 流式超时
  "api_keys": [...],              // API Key 池配置
  "retry": {...},                 // 重试配置
  "circuit_breaker": {...}        // 熔断器配置
}
```

#### API Key 配置 (`api_keys`)

| 字段 | 类型 | 说明 |
|------|------|------|
| `key` | string | API Key（必填） |
| `name` | string | 账户名称（便于识别） |
| `weight` | int | 权重，在【同一 priority 组】内按比例分配请求 |
| `priority` | int | 优先级，**值越大越优先**，优先使用高 priority 账户 |
| `is_enabled` | bool | 是否启用 |
| `limits` | object | 速率限制配置 |

#### 速率限制配置 (`limits`)

| 字段 | 类型 | 说明 | 重置时机 |
|------|------|------|----------|
| `rpm` | int | 每分钟请求数 | 滑动窗口（60秒） |
| `daily` | int | 每日请求数 | UTC 00:00 |
| `window_5h` | int | 5小时请求数 | 滚动窗口 |
| `monthly` | int | 每月请求数 | UTC 每月1日 |
| `token_daily` | int | 每日Token数 | UTC 00:00 |
| `token_monthly` | int | 每月Token数 | UTC 每月1日 |

#### 重试配置 (`retry`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_retries` | int | `3` | 最大重试次数 |
| `initial_wait` | duration | `1s` | 初始等待时间 |
| `max_wait` | duration | `30s` | 最大等待时间 |
| `multiplier` | float | `2.0` | 等待时间乘数（指数退避） |

#### 熔断器配置 (`circuit_breaker`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `threshold` | int | `5` | 连续失败次数阈值，达到后触发熔断 |
| `timeout` | duration | `60s` | 熔断等待时间，之后进入半开状态 |
| `half_open_requests` | int | `1` | 半开状态测试请求数 |

**熔断器状态：**
- **Closed（关闭）**：正常状态，请求正常转发
- **Open（打开）**：熔断状态，拒绝请求，等待恢复
- **Half-Open（半开）**：恢复探测状态，允许少量请求测试

**工作流程：**
```
连续失败达 threshold 次 → Open → 等待 timeout → Half-Open → 测试请求
    ↑                                                        ↓
    └──────────────── 测试失败 ←──────────────────── 测试成功 → Closed
```

#### 重试配置 (`retry`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_retries` | int | `3` | 最大重试次数 |
| `initial_wait` | duration | `1s` | 初始等待时间 |
| `max_wait` | duration | `30s` | 最大等待时间 |
| `multiplier` | float | `2.0` | 等待时间乘数（指数退避） |

**重试触发条件：**
- HTTP 状态码：429, 500, 502, 503, 504
- 网络错误（连接超时、连接拒绝等）

**指数退避示例：**
```
第1次失败 → 等待 1s → 重试
第2次失败 → 等待 2s → 重试
第3次失败 → 返回错误
```

#### weight 与 priority 详解

**选择流程：**

```
收到请求
    ↓
1. 找出所有可用账户中 priority 值最大的
    ↓
2. 只在这些最高 priority 的账户中进行选择
    ↓
3. 在同 priority 组内按 weight 加权轮询分配
    ↓
发送请求
```

**priority（优先级）：**
- **值越大越优先**
- `priority=2` 的账户会**优先用尽**，才会使用 `priority=1` 的账户
- 用于主备账户切换、多级账户池

**weight（权重）：**
- 仅在**同一 priority 组内**生效
- 按比例分配请求：`weight=2` 和 `weight=1` 的请求比例约为 2:1

**配置示例：**

```json
// 场景1：主备模式
"api_keys": [
  {"key": "primary", "weight": 1, "priority": 2},   // 主账户，优先使用
  {"key": "backup",  "weight": 1, "priority": 1}    // 备用，主账户不可用时才用
]

// 场景2：多主负载均衡
"api_keys": [
  {"key": "fast-1", "weight": 2, "priority": 2},    // 高速组，2倍权重
  {"key": "fast-2", "weight": 1, "priority": 2},    // 高速组，1倍权重
  {"key": "slow-1", "weight": 1, "priority": 1}     // 低速组，备用
]

// 场景3：纯负载均衡（无优先级差异）
"api_keys": [
  {"key": "acc-1", "weight": 2, "priority": 1},     // 按 2:1:1 比例分配
  {"key": "acc-2", "weight": 1, "priority": 1},
  {"key": "acc-3", "weight": 1, "priority": 1}
]
```

**简单记忆：**
- `priority` 决定**用不用**（值大先用，用完才用次大的）
- `weight` 决定**用多少**（同组内按比例分配）

---

### 模型映射 (`model_mapping`)

将简短别名映射到完整模型名称：

```json
{
  "model_mapping": {
    "gpt-4": "openai/gpt-4o",           // gpt-4 → openai provider 的 gpt-4o
    "gpt-3.5-turbo": "openai/gpt-4o-mini",
    "claude-3": "anthropic/claude-3.5-sonnet"
  }
}
```

格式：`"别名": "provider名称/模型名称"`

---

### 故障转移 (`fallback`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `false` | 是否启用故障转移 |
| `strategy` | string | `sequential` | 策略：`sequential`(顺序尝试) |
| `providers` | []string | `[]` | 故障转移 Provider 顺序列表 |

**工作流程（sequential 策略）：**
```
请求到达
    ↓
尝试 providers[0] → 成功 → 返回
    ↓ 失败
尝试 providers[1] → 成功 → 返回
    ↓ 失败
尝试 providers[2] → 成功 → 返回
    ↓ 全部失败
返回最后一个错误
```

**配置示例：**
```json
{
  "fallback": {
    "enabled": true,
    "strategy": "sequential",
    "providers": ["openrouter", "openai", "groq"]
  }
}
```

**使用场景：**
- 主 Provider 宕机时自动切换到备用
- 不同 Provider 的成本/性能权衡
- 高可用性要求的生产环境

---

### 弹性机制总览

| 机制 | 作用范围 | 触发条件 | 效果 |
|------|----------|----------|------|
| **Retry** | 单次请求 | 429/5xx 错误 | 自动重试，指数退避 |
| **Circuit Breaker** | 单个账户 | 连续失败达阈值 | 账户熔断，跳过该账户 |
| **Fallback** | 整个请求 | Provider 失败 | 切换到备用 Provider |

**协同工作流程：**
```
请求 → 选择账户 → 检查 Circuit Breaker → 发送请求
                                           ↓
                                    失败 → Retry 重试
                                           ↓
                                    仍失败 → 记录失败给 Circuit Breaker
                                           ↓
                                    Fallback 切换 Provider
```

---

### 管理 API (`admin`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 是否启用管理API |
| `listen` | string | `127.0.0.1:8081` | 监听地址（建议仅本地） |
| `api_keys` | []string | `[]` | 管理API密钥 |
| `rate_limit` | int | `100` | 每分钟请求限制 |

---

### 监控指标 (`metrics`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 是否启用指标收集 |
| `prometheus.enabled` | bool | `true` | 是否启用 Prometheus 格式 |
| `prometheus.path` | string | `/metrics` | Prometheus 指标路径 |
| `json.enabled` | bool | `true` | 是否启用 JSON 格式 |
| `namespace` | string | `aiproxy` | 指标命名空间前缀 |

---

### Token 追踪 (`token_tracking`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | `true` | 是否启用 Token 统计 |
| `streaming_mode` | string | `hybrid` | 流式追踪模式 |
| `estimation_chars_per_token` | int | `4` | Token估算字符数 |
| `reconciliation_interval` | duration | `5m` | 对账间隔 |

**流式追踪模式：**
- `hybrid` - 优先使用响应 usage 字段，无法获取时估算
- `streaming` - 仅使用流式响应中的 usage 字段
- `response` - 仅使用非流式响应估算

---

### Request ID (`request_id`)

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `header_name` | string | `X-Request-ID` | Request ID 响应头名称 |
| `generate_if_missing` | bool | `true` | 请求无ID时是否自动生成 |

---

## 环境变量覆盖

所有配置项都可以通过环境变量覆盖，格式：`AIPROXY_<SECTION>_<FIELD>`

示例：
```bash
export AIPROXY_SERVER_PORT=9090
export AIPROXY_DATABASE_PATH=/data/aiproxy.db
export AIPROXY_LOGGING_LEVEL=debug
export AIPROXY_AUTH_ENABLED=true
```

---

## 最小配置示例

```json
{
  "server": {"port": 8080},
  "providers": [
    {
      "name": "openrouter",
      "api_base": "https://openrouter.ai/api/v1",
      "models": ["openai/gpt-4o-mini"],
      "api_keys": [{"key": "sk-or-xxx"}]
    }
  ]
}
```

---

## 配置验证

启动时自动验证配置，错误示例：

```
Error: config validation failed:
  - providers[0].api_keys[0].limits.rpm: must be positive
  - providers[1].name: provider name is required
```