package config

import (
	"os"
	"testing"
)

// --- loadFile ---

func TestLoadFile_LoadsValidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "voxly-cfg-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString(`{"telegram_token":"tok123","log_level":"debug","worker_count":3,"queue_size":50}`)
	f.Close()

	cfg := &Config{LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	if err := loadFile(cfg, f.Name()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TelegramToken != "tok123" {
		t.Errorf("TelegramToken: want %q, got %q", "tok123", cfg.TelegramToken)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: want %q, got %q", "debug", cfg.LogLevel)
	}
	if cfg.WorkerCount != 3 {
		t.Errorf("WorkerCount: want 3, got %d", cfg.WorkerCount)
	}
	if cfg.QueueSize != 50 {
		t.Errorf("QueueSize: want 50, got %d", cfg.QueueSize)
	}
}

func TestLoadFile_MissingFileIsIgnored(t *testing.T) {
	cfg := &Config{LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	if err := loadFile(cfg, "/nonexistent/path/config.json"); err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	// defaults must be untouched
	if cfg.WorkerCount != 5 {
		t.Errorf("WorkerCount should be unchanged, got %d", cfg.WorkerCount)
	}
}

// --- loadFlags ---

func TestLoadFlags_OverridesNonZeroValues(t *testing.T) {
	cfg := &Config{LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	loadFlags(cfg, "new-token", "warn", 8, 200)

	if cfg.TelegramToken != "new-token" {
		t.Errorf("TelegramToken: want %q, got %q", "new-token", cfg.TelegramToken)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel: want %q, got %q", "warn", cfg.LogLevel)
	}
	if cfg.WorkerCount != 8 {
		t.Errorf("WorkerCount: want 8, got %d", cfg.WorkerCount)
	}
	if cfg.QueueSize != 200 {
		t.Errorf("QueueSize: want 200, got %d", cfg.QueueSize)
	}
}

func TestLoadFlags_IgnoresZeroValues(t *testing.T) {
	cfg := &Config{TelegramToken: "original", LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	loadFlags(cfg, "", "", 0, 0)

	if cfg.TelegramToken != "original" {
		t.Errorf("TelegramToken should be unchanged, got %q", cfg.TelegramToken)
	}
	if cfg.WorkerCount != 5 {
		t.Errorf("WorkerCount should be unchanged, got %d", cfg.WorkerCount)
	}
}

// --- loadEnv ---

func TestLoadEnv_OverridesFromEnvVars(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "env-token")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("WORKER_COUNT", "12")
	t.Setenv("QUEUE_SIZE", "256")

	cfg := &Config{LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	loadEnv(cfg)

	if cfg.TelegramToken != "env-token" {
		t.Errorf("TelegramToken: want %q, got %q", "env-token", cfg.TelegramToken)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("LogLevel: want %q, got %q", "error", cfg.LogLevel)
	}
	if cfg.WorkerCount != 12 {
		t.Errorf("WorkerCount: want 12, got %d", cfg.WorkerCount)
	}
	if cfg.QueueSize != 256 {
		t.Errorf("QueueSize: want 256, got %d", cfg.QueueSize)
	}
}

func TestLoadEnv_IgnoresEmptyValues(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("WORKER_COUNT", "")

	cfg := &Config{TelegramToken: "original", LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	loadEnv(cfg)

	if cfg.TelegramToken != "original" {
		t.Errorf("TelegramToken should be unchanged, got %q", cfg.TelegramToken)
	}
	if cfg.WorkerCount != 5 {
		t.Errorf("WorkerCount should be unchanged, got %d", cfg.WorkerCount)
	}
}
