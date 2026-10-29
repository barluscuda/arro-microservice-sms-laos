package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/barlus-developer/arro-microservice-sms-laos/internal/application"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/config"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/infrastructure/postgres"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/infrastructure/telbiz"
	"github.com/barlus-developer/arro-microservice-sms-laos/internal/transport/httpapi"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer logger.Sync()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	startupCtx, startupCancel := context.WithTimeout(context.Background(), cfg.StartupTimeout)
	defer startupCancel()
	if err := db.PingContext(startupCtx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	provider := telbiz.New(cfg.TelbizClientID, cfg.TelbizClientSecret, cfg.TelbizTimeout)
	defer provider.Close()
	store := postgres.New(db)
	service := application.NewSMSService(store, provider)
	server := httpapi.New(service, store, logger, cfg.RequestBodyLimit)
	server.Server.Addr = fmt.Sprintf(":%d", cfg.Port)
	server.Server.ReadHeaderTimeout = 5 * time.Second
	server.Server.ReadTimeout = cfg.HTTPReadTimeout
	server.Server.WriteTimeout = cfg.HTTPWriteTimeout
	server.Server.IdleTimeout = cfg.HTTPIdleTimeout
	server.Server.MaxHeaderBytes = 1 << 20

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.StartServer(server.Server) }()
	logger.Info("sms service listening", zap.Int("port", cfg.Port))

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-stopCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}
