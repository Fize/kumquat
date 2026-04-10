# Portal TODO

## 高优先级

### 1. 单元测试
- [ ] 为 service 层添加单元测试（mock repository）
- [ ] 为 handler 层添加 HTTP 测试
- [ ] 添加 JWT 工具测试
- [ ] 配置 CI/CD（GitHub Actions）

### 2. Repository 层
- [ ] 创建 pkg/repository/user.go 接口
- [ ] 创建 pkg/repository/role.go 接口
- [ ] 创建 pkg/repository/module.go 接口
- [ ] 创建 pkg/repository/project.go 接口
- [ ] 重构 service 依赖 repository 接口而非 *gorm.DB

### 3. 统一错误处理
- [ ] 创建 pkg/errors/code.go 定义错误码
- [ ] 创建 pkg/errors/app_error.go 错误类型
- [ ] 改造 handler 返回统一错误格式
- [ ] 添加全局错误恢复中间件

## 中优先级

### 4. 配置优化
- [ ] 支持环境变量 ${VAR} 形式配置
- [ ] 添加 casbin_model_path 配置项
- [ ] 分离 config.yaml 为多个环境配置

### 5. 链路追踪
- [ ] 中间件添加 X-Trace-ID 支持
- [ ] 日志携带 trace_id 字段
- [ ] 集成 OpenTelemetry（可选）

### 6. API 文档
- [ ] 添加 Swagger 注解
- [ ] 生成 swagger.json
- [ ] 部署 Swagger UI

### 7. 事务处理
- [ ] service 方法添加事务支持
- [ ] 示例：创建用户时同时初始化角色

## 低优先级

### 8. 缓存层
- [ ] 添加 Redis 缓存接口
- [ ] 用户信息缓存

### 9. 消息队列
- [ ] 添加消息通知（如用户注册通知）
- [ ] 集成 RabbitMQ 或 Kafka

### 10. 微服务拆分（未来）
- [ ] 拆分 auth service 为独立服务
- [ ] 拆分 user service 为独立服务
- [ ] 添加 API Gateway

## 已完成

- [x] 使用 go-ext 框架（config, ginserver, storage, log）
- [x] 分层架构（handler/service/model）
- [x] JWT 认证
- [x] Casbin RBAC 权限
- [x] 树形模块管理
- [x] 统一响应格式

## 架构优化 - Casbin 与 Role 关联

### 问题
- Casbin 策略表(casbin_rule)和 roles 表通过字符串 name 关联，无外键约束
- 修改 Role.Name 时不会同步更新 Casbin 策略
- 缺少级联处理，可能导致权限孤立

### 改进方案
- [ ] 方案A: 用 role ID (uint) 替代 name 作为 Casbin subject
- [ ] 方案B: 移除 Casbin，使用自定义权限表替代
- [ ] 增加 Role 变更时的级联处理
- [ ] 添加 Casbin 策略一致性校验启动检查
