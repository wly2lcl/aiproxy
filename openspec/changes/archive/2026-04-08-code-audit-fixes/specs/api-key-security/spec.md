## 新增需求

### Requirement: API 密钥存储前哈希
系统 SHALL 使用 SHA256 哈希 API 密钥后再存储到内存或数据库。

#### Scenario: 初始化时账户 API 密钥哈希
- **WHEN** 使用 API 密钥 "nvapi-abc123" 创建账户
- **THEN** 存储的 APIKeyHash SHALL 是 SHA256("nvapi-abc123")，而非原始密钥

#### Scenario: 内存转储不暴露 API 密钥
- **WHEN** 账户结构体被记录日志或序列化
- **THEN** APIKeyHash 字段 SHALL 仅包含哈希值

### Requirement: API 密钥在错误消息中遮蔽
系统 SHALL 在错误消息中遮蔽 API 密钥值以防止暴露。

#### Scenario: 配置验证错误遮蔽密钥
- **WHEN** API 密钥 "nvapi-abc123" 验证失败
- **THEN** 错误消息 SHALL 不包含 "nvapi-abc123"，使用 "***" 或遮蔽值

#### Scenario: 重复密钥错误遮蔽密钥
- **WHEN** 检测到重复 API 密钥 "nvapi-xyz789"
- **THEN** 错误消息 SHALL 不包含原始密钥值

### Requirement: 配置文件排除原始 API 密钥
系统 SHALL 支持通过环境变量而非配置文件配置 API 密钥。

#### Scenario: API 密钥来自环境变量
- **WHEN** 环境变量 APIPROXY_NVIDIA_KEY 已设置
- **THEN** 配置 SHALL 使用环境变量值，而非配置文件值

#### Scenario: 示例配置文件使用占位符
- **WHEN** 提供 config.json 示例
- **THEN** API 密钥字段 SHALL 使用 "${NVIDIA_API_KEY}" 占位符语法

### Requirement: 管理 API 密钥创建仅返回一次
系统 SHALL 仅在创建响应中返回创建的 API 密钥值。

#### Scenario: 创建响应返回密钥
- **WHEN** 管理员通过 API 创建新 API 密钥
- **THEN** 响应 SHALL 包含原始密钥值供一次性捕获

#### Scenario: 创建后密钥不可检索
- **WHEN** 管理员查询现有 API 密钥
- **THEN** 响应 SHALL 不包含原始密钥值，仅哈希标识符