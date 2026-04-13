package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fize/go-ext/config"
	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/go-ext/storage"
	"github.com/fize/kumquat/portal/pkg/handler"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/migration"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func main() {
	// 1. 加载配置
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	log.Info("starting portal server")

	// 2. 初始化数据库
	db, err := initDB(cfg)
	if err != nil {
		log.Fatal("failed to connect database", "err", err)
	}
	log.Info("database connected", "type", cfg.SQL.Type, "host", cfg.SQL.Host)

	// 3. 执行数据库迁移
	if err := migration.Migrate(db); err != nil {
		log.Fatal("failed to migrate database", "err", err)
	}
	log.Info("database migrated")

	// 4. 初始化预定义角色和权限
	roleService := service.NewRoleService(db)
	if err := roleService.InitRoles(); err != nil {
		log.Fatal("failed to initialize roles", "err", err)
	}
	log.Info("roles and permissions initialized")

	// 5. 设置 JWT 配置
	utils.JWTSecret = []byte(cfg.JWT.Secret)
	if cfg.JWT.ExpireDuration != "" {
		if d, err := time.ParseDuration(cfg.JWT.ExpireDuration); err == nil {
			utils.TokenExpireDuration = d
		}
	}
	if cfg.JWT.ResetExpireDuration != "" {
		if d, err := time.ParseDuration(cfg.JWT.ResetExpireDuration); err == nil {
			utils.ResetTokenExpireDuration = d
		}
	}

	// 6. 创建 HTTP 服务（NewServer 自动注册 TraceID + GinLogger + GinRecovery）
	server, err := ginserver.NewServer(&cfg.BaseConfig)
	if err != nil {
		log.Fatal("failed to create server", "err", err)
	}

	// 7. 额外注册 CORS 中间件
	server.Engine.Use(middleware.CORS())

	// 8. 注册路由
	registerRoutes(server.Engine, db, roleService)

	// 9. 启动服务（带优雅关闭）
	ctx, cancel, err := server.RunWithContext()
	if err != nil {
		log.Fatal("failed to run server", "err", err)
	}
	defer cancel()

	log.Info("portal server started", "addr", cfg.Server.BindAddr)

	// 10. 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Info("shutdown signal received")
	case <-ctx.Done():
		log.Info("server context done")
	}
}

// PortalConfig Portal 业务配置
type PortalConfig struct {
	config.BaseConfig
	JWT struct {
		Secret              string `mapstructure:"secret"`
		ExpireDuration      string `mapstructure:"expire_duration"`
		ResetExpireDuration string `mapstructure:"reset_expire_duration"`
	} `mapstructure:"jwt"`
	Security struct {
		AllowedEmailDomains []string `mapstructure:"allowed_email_domains"`
	} `mapstructure:"security"`
}

// loadConfig 加载配置
func loadConfig() (*PortalConfig, error) {
	cfg := &PortalConfig{
		BaseConfig: *config.NewConfig(),
	}

	cfg.Server.BindAddr = ":8080"
	cfg.JWT.Secret = "change-this-secret-in-production"
	cfg.JWT.ExpireDuration = "24h"
	cfg.JWT.ResetExpireDuration = "10m"

	if err := cfg.Load("config.yaml", false); err != nil {
		log.Warn("config file not found, using defaults", "err", err)
	}

	if err := cfg.ParseCustomConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// initDB 初始化数据库
func initDB(cfg *PortalConfig) (*gorm.DB, error) {
	sqlCfg, err := config.NewSQLConfig(
		config.WithType(cfg.SQL.Type),
		config.WithHost(cfg.SQL.Host),
		config.WithUser(cfg.SQL.User),
		config.WithPassword(cfg.SQL.Password),
		config.WithDB(cfg.SQL.DB),
		config.WithMaxIdleConns(cfg.SQL.MaxIdleConns),
		config.WithMaxOpenConns(cfg.SQL.MaxOpenConns),
	)
	if err != nil {
		return nil, err
	}

	return storage.NewDB(sqlCfg)
}

// registerRoutes 注册路由
func registerRoutes(engine *gin.Engine, db *gorm.DB, roleService *service.RoleService) {
	api := engine.Group("/api/v1")

	authService := service.NewAuthService(db)
	userService := service.NewUserService(db)
	moduleService := service.NewModuleService(db)
	projectService := service.NewProjectService(db)

	authHandler := handler.NewAuthHandler(authService)
	authHandler.SetupRoutes(api)

	userHandler := handler.NewUserHandler(userService, roleService)
	userHandler.SetupRoutes(api)

	roleHandler := handler.NewRoleHandler(roleService)
	roleHandler.SetupRoutes(api)

	moduleHandler := handler.NewModuleHandler(moduleService, roleService)
	moduleHandler.SetupRoutes(api)

	projectHandler := handler.NewProjectHandler(projectService, roleService)
	projectHandler.SetupRoutes(api)
}
