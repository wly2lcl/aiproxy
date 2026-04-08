## Why

AIProxy 项目经过全面代码审计，发现了多个**关键安全漏洞**和**业务逻辑缺陷**。这些问题可能导致：
- API 密钥泄露风险（安全问题）
- 限流功能失效（业务逻辑问题）
- 内存泄漏和性能下降
- 重试机制无效导致服务不稳定

立即修复这些问题对于保障系统安全性和稳定性至关重要。

## What Changes

### 安全修复 (Critical)
- **API 密钥存储**: 修复 `main.go:313` 将原始密钥存储而非哈希值的问题
- **配置验证**: 移除验证错误消息中暴露的 API 密钥
- **配置文件**: 移除硬编码的 API 密钥，使用环境变量替代

### 业务逻辑修复 (Critical)
- **限流映射键不匹配**: 修复 selector 使用 provider name 而非 account ID 作为 limiter 键的问题
- **重试机制**: 重试失败后切换到不同账户，而非同一账户重复尝试
- **内存泄漏**: 实现周期性清理限流窗口中的过期账户条目
- **rows.Err() 检查**: 在数据库迭代后添加错误检查

### 代码质量改进 (Medium)
- **时间解析错误处理**: 处理或记录 time.Parse 的错误
- **批量操作错误处理**: 处理 UpsertAccount 批量操作中的错误
- **限流状态持久化**: 服务重启后从数据库恢复限流状态

### 架构优化 (Low Priority)
- **拆分 main.go**: 将 2069 行的 monolithic main.go 拆分为独立 handler 文件
- **移除全局变量**: 将 globalAuthTracker 改为依赖注入方式
- **可配置常量**: 将 CircuitBreakerThreshold 从硬编码改为配置项

## Capabilities

### New Capabilities
- `rate-limiter-state-sync`: 限流状态持久化和恢复机制，确保重启后状态一致
- `account-switching-retry`: 智能账户切换重试机制，失败后自动选择其他可用账户
- `memory-cleanup`: 周期性清理过期限流窗口和统计数据，防止内存无限增长

### Modified Capabilities
- `api-key-security`: 增强 API 密钥安全处理（哈希存储、错误消息脱敏）
- `rate-limiting`: 修复限流器映射键不匹配问题，确保限流正确生效

## Impact

**受影响文件**:
- `cmd/server/main.go` - 多处修复（API密钥存储、重试逻辑、账户初始化）
- `internal/limiter/rpm.go` - 添加清理机制
- `internal/limiter/daily.go` - 添加清理机制
- `internal/storage/sqlite.go` - rows.Err() 检查、时间解析错误处理
- `internal/config/validator.go` - API 密钥错误消息脱敏
- `internal/pool/selector.go` - 修复限流器键映射
- `config/config.json` - 移除硬编码密钥，使用环境变量示例

**API 影响**: 无外部 API 变化，仅内部行为修复

**依赖影响**: 无新增依赖

**系统影响**: 提升安全性、稳定性和内存管理