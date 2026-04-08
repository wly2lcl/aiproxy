## 新增需求

### Requirement: 周期清理过期限流条目
系统 SHALL 周期性清理不活跃账户的限流条目。

#### Scenario: 移除不活跃账户条目
- **WHEN** 账户超过 1 小时未发起请求
- **THEN** 其限流窗口条目 SHALL 从内存中移除

#### Scenario: 保留活跃账户条目
- **WHEN** 账户在过去 1 小时内发起过请求
- **THEN** 其限流条目 SHALL 不被清理

### Requirement: 统计收集器内存有上限
系统 SHALL 限制统计收集器延迟和 TTFT 数组的内存使用。

#### Scenario: 延迟数组上限
- **WHEN** 统计收集器延迟数组达到 10000 条
- **THEN** 新增条目时 SHALL 移除最旧的条目

#### Scenario: TTFT 数组上限
- **WHEN** 统计收集器 TTFT 数组达到 10000 条
- **THEN** 新增条目时 SHALL 移除最旧的条目

### Requirement: 清理按可配置计划运行
系统 SHALL 允许配置清理间隔和保留阈值。

#### Scenario: 清理间隔可配置
- **WHEN** 配置中 cleanup_interval 设置为 30 分钟
- **THEN** 清理 SHALL 每 30 分钟运行

#### Scenario: 保留阈值可配置
- **WHEN** 配置中 cleanup_retention_hours 设置为 2 小时
- **THEN** 超过 2 小时不活跃的条目 SHALL 被清理