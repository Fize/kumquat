package main

// @title Kumquat Portal API
// @version 1.0.0
// @description Kumquat 多集群应用管理平台 - 用户认证与权限管理 API
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@kumquat.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

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
	k8sclient "github.com/fize/kumquat/portal/pkg/client"
	"github.com/fize/kumquat/portal/pkg/handler"
	"github.com/fize/kumquat/portal/pkg/middleware"
	"github.com/fize/kumquat/portal/pkg/migration"
	"github.com/fize/kumquat/portal/pkg/repository"
	"github.com/fize/kumquat/portal/pkg/service"
	"github.com/fize/kumquat/portal/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	_ "github.com/fize/kumquat/portal/docs"
	swagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/files"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	log.Info("starting portal server")

	server, err := ginserver.NewServer(&cfg.BaseConfig)
	if err != nil {
		log.Fatal("failed to create server", "err", err)
	}
	log.Info("ginserver initialized", "metrics", cfg.Server.Metrics.Enabled, "trace", cfg.Server.Trace.Enabled)

	db, err := initDB(cfg, log.Default())
	if err != nil {
		log.Fatal("failed to connect database", "err", err)
	}
	log.Info("database connected", "type", cfg.SQL.Type, "host", cfg.SQL.Host)

	if err := migration.Migrate(db); err != nil {
		log.Fatal("failed to migrate database", "err", err)
	}
	log.Info("database migrated")

	// 初始化 Repository
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	moduleRepo := repository.NewModuleRepository(db)
	projectRepo := repository.NewProjectRepository(db)

	// 初始化 JWT Service
	expireDuration, err := time.ParseDuration(cfg.JWT.ExpireDuration)
	if err != nil {
		expireDuration = 24 * time.Hour
	}
	resetExpireDuration, err := time.ParseDuration(cfg.JWT.ResetExpireDuration)
	if err != nil {
		resetExpireDuration = 10 * time.Minute
	}
	jwtService := service.NewJWTService(cfg.JWT.Secret, expireDuration, resetExpireDuration)

	// 初始化 Service
	roleService := service.NewRoleService(roleRepo, db)
	if err := roleService.InitRoles(); err != nil {
		log.Fatal("failed to initialize roles", "err", err)
	}
	log.Info("roles and permissions initialized")

	authService := service.NewAuthService(userRepo, roleRepo, jwtService, db)
	userService := service.NewUserService(userRepo, roleRepo, db)
	moduleService := service.NewModuleService(moduleRepo, db)
	projectService := service.NewProjectService(projectRepo, db)

	// 初始化 K8s Client（用于操作 Engine CRD）
	k8sClient, err := k8sclient.NewK8sClient(&k8sclient.Config{
		KubeconfigPath: cfg.Kubernetes.KubeconfigPath,
		MasterURL:      cfg.Kubernetes.MasterURL,
	})
	if err != nil {
		log.Warn("failed to initialize k8s client, k8s resources will not be available", "err", err)
		// 不中断启动，只是 K8s 功能不可用
	} else {
		log.Info("k8s client initialized")
	}

	// 初始化 K8s 相关 Service
	var clusterService *service.ClusterService
	var applicationService *service.ApplicationService
	var workspaceService *service.WorkspaceService
	if k8sClient != nil {
		clusterService = service.NewClusterService(k8sClient.GetClient())
		applicationService = service.NewApplicationService(k8sClient.GetClient())
		workspaceService = service.NewWorkspaceService(k8sClient.GetClient())
	}

	// 初始化 Middleware
	authMiddleware := middleware.NewAuthMiddleware(jwtService)

	server.Engine.Use(middleware.CORS())

	registerRoutes(server.Engine, db, authService, userService, moduleService, projectService, roleService, authMiddleware,
		clusterService, applicationService, workspaceService)

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
	Kubernetes struct {
		KubeconfigPath string `mapstructure:"kubeconfig_path"`
		MasterURL      string `mapstructure:"master_url"`
	} `mapstructure:"kubernetes"`
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

func initDB(cfg *PortalConfig, logger *log.Logger) (*gorm.DB, error) {
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

	return storage.NewDB(sqlCfg,
		storage.WithLogger(logger),
		storage.WithDBSlowThreshold(200*time.Millisecond),
		storage.WithDBLogLevel(gormlogger.Warn),
	)
}

func registerRoutes(engine *gin.Engine, db *gorm.DB, authService *service.AuthService, userService *service.UserService, moduleService *service.ModuleService, projectService *service.ProjectService, roleService *service.RoleService, authMiddleware *middleware.AuthMiddleware,
	clusterService *service.ClusterService, applicationService *service.ApplicationService, workspaceService *service.WorkspaceService) {
	// Swagger UI
	engine.GET("/swagger/*any", swagger.WrapHandler(swaggerFiles.Handler))

	api := engine.Group("/api/v1")

	authHandler := handler.NewAuthController(authService, authMiddleware)
	authHandler.SetupRoutes(api)

	restful := &ginserver.RestfulAPI{}

	restful.Install(engine, handler.NewUserController(userService, roleService, authMiddleware))
	restful.Install(engine, handler.NewRoleController(roleService, authMiddleware))
	restful.Install(engine, handler.NewModuleController(moduleService, roleService, authMiddleware))
	restful.Install(engine, handler.NewProjectController(projectService, roleService, authMiddleware))

	// 注册 K8s 相关路由（如果 K8s client 初始化成功）
	if clusterService != nil {
		restful.Install(engine, handler.NewClusterController(clusterService, roleService, authMiddleware))
		log.Info("cluster routes registered")
	}
	if applicationService != nil {
		restful.Install(engine, handler.NewApplicationController(applicationService, roleService, authMiddleware))
		log.Info("application routes registered")
	}
	if workspaceService != nil {
		restful.Install(engine, handler.NewWorkspaceController(workspaceService, roleService, authMiddleware))
		log.Info("workspace routes registered")
	}

	registerCustomRoutes(api, moduleService, projectService, roleService, authMiddleware)
}

func registerCustomRoutes(api *gin.RouterGroup, moduleService *service.ModuleService, projectService *service.ProjectService, roleService *service.RoleService, authMiddleware *middleware.AuthMiddleware) {
	api.GET("/modules/:id/children", authMiddleware.Auth(),
		middleware.RequirePermission(roleService, "module", "read"),
		func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				utils.BadRequest(c, "invalid id")
				return
			}
			module, err := moduleService.GetByID(c.Request.Context(), uint(id))
			if err != nil {
				log.WarnContext(c.Request.Context(), "get module children failed", "id", id, "err", err)
				utils.ErrorFromErr(c, err)
				return
			}
			utils.Success(c, module.Children)
		})

	api.GET("/projects/module/:moduleId", authMiddleware.Auth(),
		middleware.RequirePermission(roleService, "project", "read"),
		func(c *gin.Context) {
			moduleId, err := strconv.ParseUint(c.Param("moduleId"), 10, 32)
			if err != nil {
				utils.BadRequest(c, "invalid module id")
				return
			}
			page, size := utils.GetPageSize(c)
			projects, total, err := projectService.ListByModule(c.Request.Context(), uint(moduleId), page, size)
			if err != nil {
				log.ErrorContext(c.Request.Context(), "list projects by module failed", "module_id", moduleId, "err", err)
				utils.ErrorFromErr(c, err)
				return
			}
			list := make([]map[string]interface{}, len(projects))
			for i, p := range projects {
				list[i] = p.ToResponse()
			}
			utils.PageSuccess(c, total, page, size, list)
		})

	api.GET("/roles/:id/permissions", authMiddleware.Auth(),
		middleware.RequirePermission(roleService, "role", "read"),
		func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil {
				utils.BadRequest(c, "invalid id")
				return
			}
			perms, err := roleService.GetPermissions(c.Request.Context(), uint(id))
			if err != nil {
				log.WarnContext(c.Request.Context(), "get role permissions failed", "id", id, "err", err)
				utils.ErrorFromErr(c, err)
				return
			}
			utils.Success(c, gin.H{"permissions": perms})
		})
}
