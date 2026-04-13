package main

import (
	"os"
	"os/signal"
	"strconv"
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
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	log.Info("starting portal server")

	db, err := initDB(cfg)
	if err != nil {
		log.Fatal("failed to connect database", "err", err)
	}
	log.Info("database connected", "type", cfg.SQL.Type, "host", cfg.SQL.Host)

	if err := migration.Migrate(db); err != nil {
		log.Fatal("failed to migrate database", "err", err)
	}
	log.Info("database migrated")

	roleService := service.NewRoleService(db)
	if err := roleService.InitRoles(); err != nil {
		log.Fatal("failed to initialize roles", "err", err)
	}
	log.Info("roles and permissions initialized")

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

	server, err := ginserver.NewServer(&cfg.BaseConfig)
	if err != nil {
		log.Fatal("failed to create server", "err", err)
	}
	log.Info("ginserver initialized", "metrics", cfg.Server.Metrics.Enabled, "trace", cfg.Server.Trace.Enabled)

	server.Engine.Use(middleware.CORS())

	registerRoutes(server.Engine, db, roleService)

	ctx, cancel, err := server.RunWithContext()
	if err != nil {
		log.Fatal("failed to run server", "err", err)
	}
	defer cancel()

	log.Info("portal server started", "addr", cfg.Server.BindAddr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Info("shutdown signal received")
	case <-ctx.Done():
		log.Info("server context done")
	}
}

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

func registerRoutes(engine *gin.Engine, db *gorm.DB, roleService *service.RoleService) {
	api := engine.Group("/api/v1")

	authService := service.NewAuthService(db)
	userService := service.NewUserService(db)
	moduleService := service.NewModuleService(db)
	projectService := service.NewProjectService(db)

	authHandler := handler.NewAuthController(authService)
	authHandler.SetupRoutes(api)

	restful := &ginserver.RestfulAPI{}

	restful.Install(engine, handler.NewUserController(userService, roleService))
	restful.Install(engine, handler.NewRoleController(roleService))
	restful.Install(engine, handler.NewModuleController(moduleService, roleService))
	restful.Install(engine, handler.NewProjectController(projectService, roleService))

	registerCustomRoutes(api, db, roleService)
}

func registerCustomRoutes(api *gin.RouterGroup, db *gorm.DB, roleService *service.RoleService) {
	moduleService := service.NewModuleService(db)
	projectService := service.NewProjectService(db)

	api.GET("/modules/:id/children", middleware.Auth(),
		middleware.RequirePermission(roleService, "module", "read"),
		func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				utils.BadRequest(c, "invalid id")
				return
			}
			module, err := moduleService.GetByID(uint(id))
			if err != nil {
				log.WarnContext(c.Request.Context(), "get module children failed", "id", id, "err", err)
				utils.NotFound(c, "module not found")
				return
			}
			utils.Success(c, module.Children)
		})

	api.GET("/projects/module/:moduleId", middleware.Auth(),
		middleware.RequirePermission(roleService, "project", "read"),
		func(c *gin.Context) {
			moduleId, err := strconv.ParseUint(c.Param("moduleId"), 10, 32)
			if err != nil {
				utils.BadRequest(c, "invalid module id")
				return
			}
			page, size := utils.GetPageSize(c)
			projects, total, err := projectService.ListByModule(uint(moduleId), page, size)
			if err != nil {
				log.ErrorContext(c.Request.Context(), "list projects by module failed", "module_id", moduleId, "err", err)
				utils.InternalError(c, err.Error())
				return
			}
			list := make([]map[string]interface{}, len(projects))
			for i, p := range projects {
				list[i] = p.ToResponse()
			}
			utils.PageSuccess(c, total, page, size, list)
		})

	api.GET("/roles/:id/permissions", middleware.Auth(),
		middleware.RequirePermission(roleService, "role", "read"),
		func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				utils.BadRequest(c, "invalid id")
				return
			}
			perms, err := roleService.GetPermissions(uint(id))
			if err != nil {
				log.WarnContext(c.Request.Context(), "get role permissions failed", "id", id, "err", err)
				utils.NotFound(c, err.Error())
				return
			}
			utils.Success(c, gin.H{"permissions": perms})
		})
}
