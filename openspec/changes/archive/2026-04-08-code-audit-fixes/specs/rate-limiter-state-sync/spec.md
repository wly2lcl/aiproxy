## 新增需求

### Requirement: 限流状态在重启后持久化
系统 SHALL 在服务启动时从数据库恢复限流状态，防止重启后限流重置。

#### Scenario: 重启后恢复日限额
- **WHEN** 服务在账户已使用 50/100 日限额后重启
- **THEN** 该账户的日计数器 SHALL 恢复为 50，而非重置为 0

#### Scenario: 重启后恢复月限额
- **WHEN** 服务在账户已使用 5000/10000 月度令牌后重启
- **THEN** 该账户的月度令牌计数器 SHALL 恢复为 5000，而非重置为 0

### Requirement: 限流状态同步到数据库
系统 SHALL 在每次 Allow() 操作后将限流计数持久化到数据库以便恢复。

#### Scenario: 允许后持久化日计数
- **WHEN** 账户通过日限流检查，当前计数为 10
- **THEN** 数据库 SHALL 更新该账户的 current_value 为 11

#### Scenario: 流式响应后持久化令牌计数
- **WHEN** 流式响应完成，使用了 500 个完成令牌
- **THEN** 数据库 SHALL 更新该账户的令牌计数

### Requirement: 内存与数据库状态一致性
系统 SHALL 保持内存限流状态与数据库记录的一致性。

#### Scenario: 数据库查询返回当前状态
- **WHEN** 调用 GetRateLimit(accountID)
- **THEN** 返回的 Max 和 Current 值 SHALL 匹配内存状态（如可用）

#### Scenario: 滑动窗口收敛
- **WHEN** RPM 限流器仅恢复计数（无时间戳）
- **THEN** 5 分钟内滑动窗口 SHALL 收敛到准确计数，随旧时间戳过期