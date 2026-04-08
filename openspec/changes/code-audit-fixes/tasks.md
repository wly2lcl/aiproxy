## 1. 安全修复（关键）

- [x] 1.1 修复 API 密钥存储 - 将 `APIKeyHash` 字段重命名为 `APIKey`，明确存储原始密钥
  > **已优化**：字段名从 `APIKeyHash` 重命名为 `APIKey`，消除命名误导。数据库列名保持 `api_key_hash` 向后兼容。Admin API 创建账户时存储原始密钥（调用上游需要）。
- [x] 1.2 在 config/validator.go 错误消息中遮蔽 API 密钥值（行 224, 227）
- [x] 1.3 在配置加载中添加 API 密钥环境变量支持
- [x] 1.4 更新 config/config.json 使用占位符语法替代原始密钥
- [x] 1.5 添加环境变量配置文档（合并到任务 8.2）

## 2. 限流器键映射修复（关键）

- [x] 2.1 修改 selector.go 接收账户到限流器的映射而非单个组合限流器
- [x] 2.2 更新 selector.Select() 从映射中按 account.ID 检查限流器
- [x] 2.3 修改 pool.NewWeightedRoundRobin() 签名接收账户-限流器映射
- [x] 2.4 更新 main.go 选择器初始化传递正确的账户-限流器映射
- [x] 2.5 在选择器中优雅处理 nil 限流器（无限流器时允许请求）
- [x] 2.6 添加选择器按账户限流的单元测试

## 3. 账户切换重试（关键）

- [x] 3.1 修改 main.go:759-846 重试循环跟踪已尝试的账户
- [x] 3.2 每次失败账户在重试前调用 pool.RecordFailure()
- [x] 3.3 每次重试通过 selectAvailableAccount() 重新选择新账户
- [x] 3.4 添加已尝试账户集合避免重复选择同一账户
- [x] 3.5 所有账户耗尽时返回适当错误
- [x] 3.6 添加账户切换重试行为的单元测试

## 4. 限流状态持久化（高优先级）

- [x] 4.1 在限流器接口添加 LoadRateLimitState(accountID) 方法
- [x] 4.2 修改 initAccountLimiter() 在启动时从数据库加载状态
- [x] 4.3 将数据库 current_value 同步到 Daily/Monthly 限流器内存计数器
- [x] 4.4 处理滑动窗口（RPM）恢复仅用计数（接受近似值）
- [x] 4.5 确保 Record() 一致更新内存和数据库
- [x] 4.6 添加模拟重启后状态恢复的单元测试

## 5. 内存清理（中优先级）

- [x] 5.1 在 RPM 限流器添加 CleanupStale(maxAge time.Duration) 方法
- [x] 5.2 在 Daily 限流器添加 CleanupStale(maxAge time.Duration) 方法
- [x] 5.3 在 Window 限流器添加 CleanupStale(maxAge time.Duration) 方法
- [x] 5.4 在 CompositeLimiter 添加 CleanupStale(maxAge time.Duration) 方法
- [x] 5.5 跟踪每个账户条目在限流器中的最后访问时间
- [x] 5.6 在现有清理 ticker 循环中添加清理调用（main.go:424-428）
- [x] 5.7 添加 stats collector 数组大小上限（最多 10000 条）
- [x] 5.8 添加清理间隔和保留时间的配置选项

## 6. 数据库错误处理（中优先级）

- [x] 6.1 在 sqlite.go:665-683（GetRequestTimeSeries）迭代后添加 rows.Err() 检查
- [x] 6.2 处理或记录 sqlite.go:679, 937-939, 982-988 的 time.Parse 错误
- [x] 6.3 处理 main.go:1774, 1784 批量操作中的 UpsertAccount 错误
- [x] 6.4 处理 sqlite.go:457 的 RowsAffected 错误
- [x] 6.5 添加重复时间解析模式的辅助函数

## 7. 测试和验证

- [x] 7.1 编写限流器键映射正确性的集成测试
- [x] 7.2 编写账户切换重试的集成测试
- [x] 7.3 编写限流状态持久化的集成测试
- [x] 7.4 编写内存清理效果的测试
- [x] 7.5 验证修改后所有现有测试通过
- [x] 7.6 运行完整测试套件并检查覆盖率

## 8. 文档

- [x] 8.1 更新 README 添加安全最佳实践章节
- [x] 8.2 文档化 API 密钥的环境变量配置
- [x] 8.3 为现有包含原始密钥的配置文件添加迁移指南
- [x] 8.4 文档化限流状态持久化行为（已在 README 中说明）