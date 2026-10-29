package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port               int
	DatabaseURL        string
	DBMaxOpenConns     int
	DBMaxIdleConns     int
	DBConnMaxLifetime  time.Duration
	TelbizClientID     string
	TelbizClientSecret string
	TelbizTimeout      time.Duration
	StartupTimeout     time.Duration
	ShutdownTimeout    time.Duration
	HTTPReadTimeout    time.Duration
	HTTPWriteTimeout   time.Duration
	HTTPIdleTimeout    time.Duration
	RequestBodyLimit   string
}

func Load() (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("PORT", 3285)
	v.SetDefault("DB_MAX_OPEN_CONNS", 20)
	v.SetDefault("DB_MAX_IDLE_CONNS", 5)
	v.SetDefault("DB_CONN_MAX_LIFETIME", "30m")
	v.SetDefault("TELBIZ_TIMEOUT", "30s")
	v.SetDefault("STARTUP_TIMEOUT", "10s")
	v.SetDefault("SHUTDOWN_TIMEOUT", "10s")
	v.SetDefault("HTTP_READ_TIMEOUT", "15s")
	v.SetDefault("HTTP_WRITE_TIMEOUT", "30s")
	v.SetDefault("HTTP_IDLE_TIMEOUT", "60s")
	v.SetDefault("REQUEST_BODY_LIMIT", "1M")

	cfg := Config{
		Port:               v.GetInt("PORT"),
		DatabaseURL:        v.GetString("DATABASE_URL"),
		DBMaxOpenConns:     v.GetInt("DB_MAX_OPEN_CONNS"),
		DBMaxIdleConns:     v.GetInt("DB_MAX_IDLE_CONNS"),
		DBConnMaxLifetime:  v.GetDuration("DB_CONN_MAX_LIFETIME"),
		TelbizClientID:     v.GetString("TELBIZ_CLIENT_ID"),
		TelbizClientSecret: v.GetString("TELBIZ_CLIENT_SECRET"),
		TelbizTimeout:      v.GetDuration("TELBIZ_TIMEOUT"),
		StartupTimeout:     v.GetDuration("STARTUP_TIMEOUT"),
		ShutdownTimeout:    v.GetDuration("SHUTDOWN_TIMEOUT"),
		HTTPReadTimeout:    v.GetDuration("HTTP_READ_TIMEOUT"),
		HTTPWriteTimeout:   v.GetDuration("HTTP_WRITE_TIMEOUT"),
		HTTPIdleTimeout:    v.GetDuration("HTTP_IDLE_TIMEOUT"),
		RequestBodyLimit:   v.GetString("REQUEST_BODY_LIMIT"),
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("PORT must be between 1 and 65535")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.TelbizClientID == "" || cfg.TelbizClientSecret == "" {
		return Config{}, fmt.Errorf("TELBIZ_CLIENT_ID and TELBIZ_CLIENT_SECRET are required")
	}
	if cfg.DBMaxOpenConns <= 0 || cfg.DBMaxIdleConns < 0 || cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		return Config{}, fmt.Errorf("database connection pool settings are invalid")
	}
	if cfg.TelbizTimeout <= 0 || cfg.StartupTimeout <= 0 || cfg.ShutdownTimeout <= 0 || cfg.HTTPReadTimeout <= 0 || cfg.HTTPWriteTimeout <= 0 || cfg.HTTPIdleTimeout <= 0 {
		return Config{}, fmt.Errorf("configured timeouts must be positive")
	}
	return cfg, nil
}
