## 新增需求

### Requirement: 限流器使用账户 ID 作为键
系统 SHALL 使用账户 ID 作为限流器查找键，而非提供商名称。

#### Scenario: 限流器按账户 ID 存储
- **WHEN** ID 为 "provider1:key1" 的账户 A 初始化
- **THEN** 其限流器 SHALL 存储在 limiters["provider1:key1"]，而非 limiters["provider1"]

#### Scenario: 选择器按账户 ID 获取限流器
- **WHEN** 选择器选择 ID 为 "provider1:key1" 的账户 A
- **THEN** 限流检查 SHALL 使用 limiters["provider1:key1"]，而非提供商级限流器

### Requirement: 每个账户有独立限流
系统 SHALL 按账户独立执行限流，而非按提供商。

#### Scenario: 同提供商下两个账户有独立限额
- **WHEN** 提供商 P 有账户 A1（限额 100/天）和 A2（限额 50/天）
- **THEN** A1 使用 80 次请求 SHALL 不影响 A2 的剩余配额

#### Scenario: 账户限额耗尽，其他账户可用
- **WHEN** 账户 A1 达到日限额
- **THEN** 同提供商下的账户 A2 SHALL 仍可被选择（如限额未达）

### Requirement: 组合限流器按账户传递给选择器
系统 SHALL 将账户特定的组合限流器传递给选择器以正确检查限流。

#### Scenario: 选择器接收账户-限流器映射
- **WHEN** 选择器初始化
- **THEN** 它 SHALL 接收账户 ID 到组合限流器的映射

#### Scenario: 选择器为每个候选账户检查限流器
- **WHEN** 选择器评估账户 A 以选择
- **THEN** 它 SHALL 检查 limiters[A.ID].Allow()，而非共享的提供商限流器

### Requirement: 限流器查找缺失时优雅返回
系统 SHALL 无崩溃地处理缺失的限流器条目。

#### Scenario: 无账户限流器时返回通过
- **WHEN** 账户 A 无配置限额（limiters[A.ID] 为 nil）
- **THEN** 限流检查 SHALL 通过（允许请求）

#### Scenario: 限流器初始化为所有账户创建条目
- **WHEN** 从配置初始化账户
- **THEN** 即使限额为零， SHALL 为每个账户创建限流器