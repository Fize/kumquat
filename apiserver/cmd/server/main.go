package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fize/go-ext/config"
	"github.com/fize/go-ext/ginserver"
	"github.com/fize/go-ext/log"
	"github.com/fize/go-ext/storage"
	k8sclient "github.com/fize/kumquat/apiserver/pkg/client"
	"github.com/fize/kumquat/apiserver/pkg/dto"
	"github.com/fize/kumquat/apiserver/pkg/handler"
	"github.com/fize/kumquat/apiserver/pkg/middleware"
	"github.com/fize/kumquat/apiserver/pkg/migration"
	"github.com/fize/kumquat/apiserver/pkg/model"
	"github.com/fize/kumquat/apiserver/pkg/repository"
	"github.com/fize/kumquat/apiserver/pkg/service"
	"github.com/fize/kumquat/apiserver/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal("failed to load config", "err", err)
	}

	if err := run(cfg); err != nil {
		log.Fatal("server error", "err", err)
	}
}

func run(cfg *APIConfig) error {
	log.Info("starting api server")

	server, err := ginserver.NewServer(&cfg.BaseConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	log.Info("ginserver initialized", "metrics", cfg.Server.Metrics.Enabled, "trace", cfg.Server.Trace.Enabled)

	db, err := initDB(cfg, log.Default())
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}
	defer func() {
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}()
	log.Info("database connected", "type", cfg.SQL.Type, "host", cfg.SQL.Host)

	if err := migration.Migrate(db); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}
	log.Info("database migrated")

	// Initialize Repository
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	moduleRepo := repository.NewModuleRepository(db)
	projectRepo := repository.NewProjectRepository(db)

	// Initialize JWT Service
	expireDuration, err := time.ParseDuration(cfg.JWT.ExpireDuration)
	if err != nil {
		expireDuration = 24 * time.Hour
	}
	resetExpireDuration, err := time.ParseDuration(cfg.JWT.ResetExpireDuration)
	if err != nil {
		resetExpireDuration = 10 * time.Minute
	}
	jwtService := service.NewJWTService(cfg.JWT.Secret, expireDuration, resetExpireDuration)

	// Initialize Service
	roleService := service.NewRoleService(roleRepo, db)
	if err := roleService.InitRoles(); err != nil {
		return fmt.Errorf("failed to initialize roles: %w", err)
	}
	log.Info("roles and permissions initialized")

	authService := service.NewAuthService(userRepo, roleRepo, jwtService, db)
	userService := service.NewUserService(userRepo, roleRepo, db)

	if bootstrapPassword := os.Getenv("KUMQUAT_BOOTSTRAP_ADMIN_PASSWORD"); bootstrapPassword != "" {
		username := os.Getenv("KUMQUAT_BOOTSTRAP_ADMIN_USERNAME")
		if username == "" {
			username = "admin"
		}
		email := os.Getenv("KUMQUAT_BOOTSTRAP_ADMIN_EMAIL")
		if email == "" {
			email = username + "@kumquat.local"
		}
		existing, lookupErr := userRepo.GetByUsername(context.TODO(), username)
		switch {
		case errors.Is(lookupErr, gorm.ErrRecordNotFound):
			if err := authService.InitAdminUser(context.TODO(), username, email, bootstrapPassword); err != nil {
				return fmt.Errorf("failed to initialize configured admin user: %w", err)
			}
		case lookupErr != nil:
			return fmt.Errorf("failed to inspect configured admin user: %w", lookupErr)
		default:
			adminRole, roleErr := roleRepo.GetByName(context.TODO(), model.RoleAdmin)
			if roleErr != nil || existing.RoleID != adminRole.ID || existing.Email != email {
				return fmt.Errorf("configured bootstrap username is already claimed by another identity")
			}
		}
	}
	moduleService := service.NewModuleService(moduleRepo, db)
	projectService := service.NewProjectService(projectRepo, db)

	// Initialize K8s Client
	k8sClient, err := k8sclient.NewK8sClient(&k8sclient.Config{
		KubeconfigPath: cfg.Kubernetes.KubeconfigPath,
		MasterURL:      cfg.Kubernetes.MasterURL,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize required engine client: %w", err)
	} else {
		log.Info("k8s client initialized")
	}

	// Initialize Middleware
	authMiddleware := middleware.NewAuthMiddleware(jwtService)

	server.Engine.Use(middleware.Recovery())
	server.Engine.Use(middleware.CORS(cfg.Security.AllowedOrigins))

	// Rate limiting: defaults to 100 req/s with burst of 200 if not configured
	rateLimit := cfg.Security.RateLimit
	if rateLimit <= 0 {
		rateLimit = 100
	}
	rateLimitBurst := cfg.Security.RateLimitBurst
	if rateLimitBurst <= 0 {
		rateLimitBurst = 200
	}
	server.Engine.Use(middleware.RateLimit(rateLimit, rateLimitBurst))

	resourceGateway := service.NewResourceGateway(db, k8sClient.GetClient())
	registerRoutes(server.Engine, db, authService, userService, moduleService, projectService, roleService, authMiddleware, resourceGateway)

	ctx, cancel, err := server.RunWithContext()
	if err != nil {
		return fmt.Errorf("failed to run server: %w", err)
	}
	defer cancel()
	go resourceGateway.Run(ctx)

	log.Info("api server started", "addr", cfg.Server.BindAddr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Info("shutdown signal received")
	case <-ctx.Done():
		log.Info("server context done")
	}

	log.Info("server shutdown complete")
	return nil
}

type APIConfig struct {
	config.BaseConfig
	JWT struct {
		Secret              string `mapstructure:"secret"`
		ExpireDuration      string `mapstructure:"expire_duration"`
		ResetExpireDuration string `mapstructure:"reset_expire_duration"`
	} `mapstructure:"jwt"`
	Security struct {
		AllowedEmailDomains []string `mapstructure:"allowed_email_domains"`
		AllowedOrigins      []string `mapstructure:"allowed_origins"`
		RateLimit           float64  `mapstructure:"rate_limit"`
		RateLimitBurst      int      `mapstructure:"rate_limit_burst"`
	} `mapstructure:"security"`
	Kubernetes struct {
		KubeconfigPath string `mapstructure:"kubeconfig_path"`
		MasterURL      string `mapstructure:"master_url"`
	} `mapstructure:"kubernetes"`
}

func loadConfig() (*APIConfig, error) {
	cfg := &APIConfig{
		BaseConfig: *config.NewConfig(),
	}

	cfg.Server.BindAddr = ":8080"
	// JWT Secret: REQUIRED — must be set via config file (config.yaml) or environment variable.
	// If not set, the server will exit with an error.
	cfg.JWT.ExpireDuration = "24h"
	cfg.JWT.ResetExpireDuration = "10m"

	if err := cfg.Load("config.yaml", false); err != nil {
		log.Warn("config file not found, using defaults", "err", err)
	}

	if err := cfg.ParseCustomConfig(cfg); err != nil {
		return nil, err
	}
	if value := os.Getenv("KUMQUAT_API_JWT_SECRET"); value != "" {
		cfg.JWT.Secret = value
	}
	if value := os.Getenv("KUMQUAT_API_KUBECONFIG"); value != "" {
		cfg.Kubernetes.KubeconfigPath = value
	}
	if value := os.Getenv("KUMQUAT_API_MASTER_URL"); value != "" {
		cfg.Kubernetes.MasterURL = value
	}
	if value := os.Getenv("KUMQUAT_API_SQL_TYPE"); value != "" {
		cfg.SQL.Type = value
	}
	if value := os.Getenv("KUMQUAT_API_SQL_HOST"); value != "" {
		cfg.SQL.Host = value
	}
	if value := os.Getenv("KUMQUAT_API_SQL_USER"); value != "" {
		cfg.SQL.User = value
	}
	if value := os.Getenv("KUMQUAT_API_SQL_PASSWORD"); value != "" {
		cfg.SQL.Password = value
	}
	if value := os.Getenv("KUMQUAT_API_SQL_DB"); value != "" {
		cfg.SQL.DB = value
	}
	if err := applyPositiveIntEnv("KUMQUAT_API_SQL_MAX_IDLE_CONNS", &cfg.SQL.MaxIdleConns); err != nil {
		return nil, err
	}
	if err := applyPositiveIntEnv("KUMQUAT_API_SQL_MAX_OPEN_CONNS", &cfg.SQL.MaxOpenConns); err != nil {
		return nil, err
	}

	if cfg.JWT.Secret == "" {
		return nil, fmt.Errorf("JWT secret is not configured; set custom.jwt.secret in config.yaml or KUMQUAT_API_JWT_SECRET")
	}

	return cfg, nil
}

func applyPositiveIntEnv(name string, target *int) error {
	value := os.Getenv(name)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*target = parsed
	return nil
}

func initDB(cfg *APIConfig, logger *log.Logger) (*gorm.DB, error) {
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

func registerRoutes(engine *gin.Engine, db *gorm.DB, authService *service.AuthService, userService *service.UserService, moduleService *service.ModuleService, projectService *service.ProjectService, roleService *service.RoleService, authMiddleware *middleware.AuthMiddleware, resourceGateway *service.ResourceGateway) {
	// Health check endpoint
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	engine.GET("/readyz", func(c *gin.Context) {
		if err := resourceGateway.Ready(c.Request.Context()); err != nil {
			c.JSON(503, gin.H{"status": "unavailable", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})

	api := engine.Group("/api/v1")

	authHandler := handler.NewAuthController(authService, authMiddleware)
	authHandler.SetupRoutes(api)

	resourceController := handler.NewResourceGatewayController(resourceGateway)
	principal := resourceController.Principal()
	registerAdministrativeRoutes(api, userService, moduleService, projectService, roleService, authMiddleware, principal)

	resourceController.Register(api, authMiddleware.Auth(), func(resource, action string) gin.HandlerFunc {
		return middleware.RequirePermission(roleService, resource, action)
	})
	registerCustomRoutes(api, moduleService, projectService, roleService, authMiddleware, principal)
}

func resolveHandler(fn func() (gin.HandlerFunc, error)) gin.HandlerFunc {
	h, err := fn()
	if err != nil {
		panic(err)
	}
	return h
}

func registerAdministrativeRoutes(api *gin.RouterGroup, userSvc *service.UserService, moduleSvc *service.ModuleService, projectSvc *service.ProjectService, roleSvc *service.RoleService, auth *middleware.AuthMiddleware, principal gin.HandlerFunc) {
	read := func(resource string) []gin.HandlerFunc {
		return []gin.HandlerFunc{auth.Auth(), principal, middleware.RequirePermission(roleSvc, resource, model.ActionRead)}
	}
	admin := []gin.HandlerFunc{auth.Auth(), principal, middleware.RequireRole(model.RoleAdmin)}
	users := handler.NewUserController(userSvc, roleSvc, auth)
	api.GET("/users", append(read(model.ResourceUser), resolveHandler(users.List))...)
	api.POST("/users", append(admin, resolveHandler(users.Create))...)
	api.GET("/users/:id", append(read(model.ResourceUser), resolveHandler(users.Get))...)
	api.PUT("/users/:id", append(admin, resolveHandler(users.Update))...)
	api.DELETE("/users/:id", append(admin, resolveHandler(users.Delete))...)
	roles := handler.NewRoleController(roleSvc, auth)
	api.GET("/roles", append(admin, resolveHandler(roles.List))...)
	api.GET("/roles/:id", append(admin, resolveHandler(roles.Get))...)
	modules := handler.NewModuleController(moduleSvc, roleSvc, auth)
	api.GET("/modules", append(read(model.ResourceModule), resolveHandler(modules.List))...)
	api.POST("/modules", append(admin, resolveHandler(modules.Create))...)
	api.GET("/modules/:id", append(read(model.ResourceModule), resolveHandler(modules.Get))...)
	api.PUT("/modules/:id", append(admin, resolveHandler(modules.Update))...)
	api.DELETE("/modules/:id", append(admin, resolveHandler(modules.Delete))...)
	projects := handler.NewProjectController(projectSvc, roleSvc, auth)
	api.GET("/projects", append(read(model.ResourceProject), resolveHandler(projects.List))...)
	api.POST("/projects", append(admin, resolveHandler(projects.Create))...)
	api.GET("/projects/:id", append(read(model.ResourceProject), resolveHandler(projects.Get))...)
	api.PUT("/projects/:id", append(admin, resolveHandler(projects.Update))...)
	api.DELETE("/projects/:id", append(admin, resolveHandler(projects.Delete))...)
}

func registerCustomRoutes(api *gin.RouterGroup, moduleService *service.ModuleService, projectService *service.ProjectService, roleService *service.RoleService, authMiddleware *middleware.AuthMiddleware, principalMiddleware gin.HandlerFunc) {
	api.GET("/modules/:id/children", authMiddleware.Auth(), principalMiddleware,
		middleware.RequirePermission(roleService, "module", "read"),
		func(c *gin.Context) {
			id := c.Param("id")
			principal := service.PrincipalFromContext(c.Request.Context())
			if !principal.Admin {
				allowed, scopeErr := moduleService.CanAccess(c.Request.Context(), principal.ModulePublicID, id)
				if scopeErr != nil {
					utils.InternalError(c, "failed to evaluate business scope")
					return
				}
				if !allowed {
					utils.Forbidden(c, service.ErrForbidden.Error())
					return
				}
			}
			children, err := moduleService.GetChildrenByPublicID(c.Request.Context(), id)
			if err != nil {
				log.WarnContext(c.Request.Context(), "get module children failed", "id", id, "err", err)
				utils.ErrorFromErr(c, err)
				return
			}
			list := make([]dto.ModuleDTO, len(children))
			for i := range children {
				list[i] = dto.ModuleFromModel(children[i])
			}
			utils.Success(c, list)
		})

	api.GET("/projects/module/:moduleId", authMiddleware.Auth(), principalMiddleware,
		middleware.RequirePermission(roleService, "project", "read"),
		func(c *gin.Context) {
			moduleId := c.Param("moduleId")
			principal := service.PrincipalFromContext(c.Request.Context())
			if !principal.Admin {
				allowed, scopeErr := moduleService.CanAccess(c.Request.Context(), principal.ModulePublicID, moduleId)
				if scopeErr != nil {
					utils.InternalError(c, "failed to evaluate business scope")
					return
				}
				if !allowed {
					utils.Forbidden(c, service.ErrForbidden.Error())
					return
				}
			}
			page, size := utils.GetPageSize(c)
			projects, total, err := projectService.ListByModulePublicID(c.Request.Context(), moduleId, page, size)
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

	api.GET("/roles/:id/permissions", authMiddleware.Auth(), middleware.RequireRole(model.RoleAdmin),
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
			permissions := make([]dto.PermissionDTO, len(perms))
			for i := range perms {
				permissions[i] = dto.PermissionFromModel(perms[i])
			}
			utils.Success(c, gin.H{"permissions": permissions})
		})

}
