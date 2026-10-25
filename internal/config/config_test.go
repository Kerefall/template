package config

import (
	"os"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()

	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("DATABASE_URL", "test-database-url")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("DATABASE_MAX_CONNS", "10")
	t.Setenv("DATABASE_MIN_CONNS", "2")
	t.Setenv("DATABASE_MAX_CONN_LIFETIME", "30m")
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "5s")
	t.Setenv("DATABASE_QUERY_TIMEOUT", "3s")
}

func TestLoadConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HTTP_READ_TIMEOUT", "11s")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "6s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "12s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "70s")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("expected HTTP_ADDR to be :8080, got %s", cfg.HTTPAddr)
	}
	if cfg.DatabaseMaxConns != 10 {
		t.Errorf("expected DatabaseMaxConns to be 10, got %d", cfg.DatabaseMaxConns)
	}
	if cfg.HTTPReadTimeout != 11*time.Second {
		t.Errorf("expected HTTPReadTimeout to be 11s, got %s", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPIdleTimeout != 70*time.Second {
		t.Errorf("expected HTTPIdleTimeout to be 70s, got %s", cfg.HTTPIdleTimeout)
	}
}

func TestLoadConfigHTTPTimeoutDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTTPReadTimeout != 10*time.Second ||
		cfg.HTTPReadHeaderTimeout != 5*time.Second ||
		cfg.HTTPWriteTimeout != 10*time.Second ||
		cfg.HTTPIdleTimeout != 60*time.Second {
		t.Errorf("unexpected default HTTP timeouts: %+v", cfg)
	}
}

func TestLoadConfigMissingRequired(t *testing.T) {
	setRequiredEnv(t)
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected an error when DATABASE_URL is not set")
	}
}

func TestLoadConfigInvalidValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DATABASE_MIN_CONNS", "20")

	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected an error when DATABASE_MIN_CONNS > DATABASE_MAX_CONNS")
	}
}
