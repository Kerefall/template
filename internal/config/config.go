package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr                string
	LogLevel                string
	ShutdownTimeout         time.Duration
	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration

	HTTPReadTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
}

func (c Config) validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("LOG_LEVEL must not be empty")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL must not be empty")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be positive")
	}
	if c.DatabaseMaxConns <= 0 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be positive")
	}
	if c.DatabaseMinConns < 0 || c.DatabaseMinConns > c.DatabaseMaxConns {
		return fmt.Errorf("DATABASE_MIN_CONNS must be between 0 and DATABASE_MAX_CONNS")
	}
	if c.DatabaseMaxConnLifetime <= 0 {
		return fmt.Errorf("DATABASE_MAX_CONN_LIFETIME must be positive")
	}
	if c.DatabaseConnectTimeout <= 0 {
		return fmt.Errorf("DATABASE_CONNECT_TIMEOUT must be positive")
	}
	if c.DatabaseQueryTimeout <= 0 {
		return fmt.Errorf("DATABASE_QUERY_TIMEOUT must be positive")
	}
	if c.HTTPReadTimeout <= 0 {
		return fmt.Errorf("HTTP_READ_TIMEOUT must be positive")
	}
	if c.HTTPReadHeaderTimeout <= 0 {
		return fmt.Errorf("HTTP_READ_HEADER_TIMEOUT must be positive")
	}
	if c.HTTPWriteTimeout <= 0 {
		return fmt.Errorf("HTTP_WRITE_TIMEOUT must be positive")
	}
	if c.HTTPIdleTimeout <= 0 {
		return fmt.Errorf("HTTP_IDLE_TIMEOUT must be positive")
	}

	return nil
}

func LoadConfig() (Config, error) {
	cfg := Config{}

	value, ok := os.LookupEnv("HTTP_ADDR")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable HTTP_ADDR is not set")
	}
	cfg.HTTPAddr = value

	value, ok = os.LookupEnv("LOG_LEVEL")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable LOG_LEVEL is not set")
	}
	cfg.LogLevel = value

	value, ok = os.LookupEnv("SHUTDOWN_TIMEOUT")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable SHUTDOWN_TIMEOUT is not set")
	}

	shutdownTimeout, err := time.ParseDuration(value)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}
	cfg.ShutdownTimeout = shutdownTimeout

	value, ok = os.LookupEnv("DATABASE_URL")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable DATABASE_URL is not set")
	}
	cfg.DatabaseURL = value

	value, ok = os.LookupEnv("DATABASE_MAX_CONNS")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable DATABASE_MAX_CONNS is not set")
	}

	maxConns, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_MAX_CONNS: %w", err)
	}
	cfg.DatabaseMaxConns = int32(maxConns)

	value, ok = os.LookupEnv("DATABASE_MIN_CONNS")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable DATABASE_MIN_CONNS is not set")
	}

	databaseMinConns, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_MIN_CONNS: %w", err)
	}
	cfg.DatabaseMinConns = int32(databaseMinConns)

	value, ok = os.LookupEnv("DATABASE_MAX_CONN_LIFETIME")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable DATABASE_MAX_CONN_LIFETIME is not set")
	}

	databaseMaxConnLifetime, err := time.ParseDuration(value)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_MAX_CONN_LIFETIME: %w", err)
	}
	cfg.DatabaseMaxConnLifetime = databaseMaxConnLifetime

	value, ok = os.LookupEnv("DATABASE_CONNECT_TIMEOUT")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable DATABASE_CONNECT_TIMEOUT is not set")
	}

	databaseConnectTimeout, err := time.ParseDuration(value)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_CONNECT_TIMEOUT: %w", err)
	}
	cfg.DatabaseConnectTimeout = databaseConnectTimeout

	value, ok = os.LookupEnv("DATABASE_QUERY_TIMEOUT")
	if !ok {
		return Config{}, fmt.Errorf("required environment variable DATABASE_QUERY_TIMEOUT is not set")
	}

	databaseQueryTimeout, err := time.ParseDuration(value)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_QUERY_TIMEOUT: %w", err)
	}
	cfg.DatabaseQueryTimeout = databaseQueryTimeout

	// Таймауты HTTP-сервера необязательны: у них есть безопасные значения по умолчанию,
	// потому что tripgoctl кладёт в .env только переменные базы и сервера
	if cfg.HTTPReadTimeout, err = optionalDuration("HTTP_READ_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadHeaderTimeout, err = optionalDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = optionalDuration("HTTP_WRITE_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = optionalDuration("HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// optionalDuration читает длительность из окружения; если переменной нет
// или она пустая, возвращает значение по умолчанию
func optionalDuration(key string, def time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return def, nil
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return d, nil
}
