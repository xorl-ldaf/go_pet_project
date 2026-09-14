package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	authhttp "go_pet_project/internal/auth/adapter/in/http"
	authjwt "go_pet_project/internal/auth/adapter/out/jwt"
	authpassword "go_pet_project/internal/auth/adapter/out/password"
	authpostgres "go_pet_project/internal/auth/adapter/out/postgres"
	authrefresh "go_pet_project/internal/auth/adapter/out/refresh"
	authservice "go_pet_project/internal/auth/application/service"
	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/logging"
	"go_pet_project/internal/platform/server"
	userhttp "go_pet_project/internal/user/adapter/in/http"
	userpostgres "go_pet_project/internal/user/adapter/out/postgres"
	userservice "go_pet_project/internal/user/application/service"
)

type API struct {
	cfg    *config.Config
	logger *slog.Logger
	db     *database.Postgres
	server *server.Server
}

func NewAPI(ctx context.Context, cfg *config.Config) (*API, error) {
	logger := logging.New()

	db, err := database.Open(ctx, cfg.DB, logger)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	userRepository := userpostgres.NewRepository(db.GORM)
	refreshTokenRepository := authpostgres.NewRefreshTokenRepository(db.GORM)

	passwordHasher, err := authpassword.NewHasher(authpassword.DefaultCost)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create password hasher: %w", err)
	}

	tokenProvider, err := authjwt.NewProvider(cfg.Auth.JWTSecret, cfg.Auth.AccessTokenTTL)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create token provider: %w", err)
	}

	refreshTokenGenerator, err := authrefresh.NewTokenGenerator(authrefresh.DefaultTokenBytes)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create refresh token generator: %w", err)
	}

	authService, err := authservice.NewAuthService(
		userRepository,
		passwordHasher,
		tokenProvider,
		refreshTokenRepository,
		refreshTokenGenerator,
		cfg.Auth.RefreshTokenTTL,
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create auth service: %w", err)
	}

	authHandler := authhttp.NewHandler(authService, logger)

	authMiddleware, err := authhttp.NewAuthMiddleware(tokenProvider)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create auth middleware: %w", err)
	}

	userService, err := userservice.NewUserService(userRepository)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create user service: %w", err)
	}

	userHandler := userhttp.NewHandler(userService, logger)

	router := server.NewRouter(func(ctx context.Context) error {
		return db.Ping(ctx)
	}, func(mux *http.ServeMux) {
		authhttp.RegisterRoutes(mux, authHandler)
		userhttp.RegisterRoutes(mux, userHandler, authMiddleware.Authenticate)
	})

	return &API{
		cfg:    cfg,
		logger: logger,
		db:     db,
		server: server.New(cfg.HTTP.Address(), router, logger),
	}, nil
}

func (a *API) Start() error {
	a.logger.Info("application startup", "http_port", a.cfg.HTTP.Port, "database", a.cfg.DB.RedactedURL())

	return a.server.Start()
}

func (a *API) Shutdown(ctx context.Context) error {
	a.logger.Info("application shutdown started")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	if err := a.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	a.logger.Info("application shutdown completed")

	return nil
}
