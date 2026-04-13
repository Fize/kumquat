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
- [ ] 分离 config.yaml 为多个环境配置

### 5. 链路追踪
- [ ] 中间件添加 X-Trace-ID 支持（注：go-ext ginserver 已自动注册 TraceID 中间件）
- [ ] 日志携带 trace_id 字段（注：ginserver 自动处理）
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
- [x] 自定义 ACL 权限（Permission 表替代 Casbin）
- [x] 树形模块管理
- [x] 统一响应格式

## ACL 权限扩展说明

### 当前设计（阶段1）
- Role → Permission（一对多，通过 role_id 外键关联）
- Permission 表字段：id, role_id(FK), resource, action, effect
- 预定义角色权限：
  - admin → (*, *, allow)
  - member → (module, read), (project, *), (user, read), (role, read)
  - guest → (*, read)
- 粗粒度鉴权：`RequireRole("admin")` 直接比对角色名
- 细粒度鉴权：`RequirePermission(roleService, "module", "read")` 查 Permission 表

### 阶段2 扩展（用户级权限覆盖）
- 在 Permission 表加 `user_id *uint` 列
- 用户级权限优先级高于角色级权限
- 实现：查询时优先查 user_id 匹配的规则，无则查 role_id 规则

### 阶段3 扩展（资源级权限）
- 在 Permission 表加 `resource_id *string` 列
- 支持"只能访问模块A下的项目"等细粒度控制
