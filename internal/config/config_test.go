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
	loadFlags(cfg, flagValues{
		token:            "new-token",
		logLevel:         "warn",
		workerCount:      8,
		queueSize:        200,
		dbDSN:            "postgres://localhost/voxly",
		dbMigrationsPath: "file://migrations",
		dbMaxOpenConns:   20,
		ssAuthKey:        "ss-auth-key-b64",
		ssScope:          "SALUTE_SPEECH_PERS",
	})

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
	if cfg.DatabaseDSN != "postgres://localhost/voxly" {
		t.Errorf("DatabaseDSN: want %q, got %q", "postgres://localhost/voxly", cfg.DatabaseDSN)
	}
	if cfg.DBMaxOpenConns != 20 {
		t.Errorf("DBMaxOpenConns: want 20, got %d", cfg.DBMaxOpenConns)
	}
	if cfg.SaluteSpeechAuthorizationKey != "ss-auth-key-b64" {
		t.Errorf("SaluteSpeechAuthorizationKey: want %q, got %q", "ss-auth-key-b64", cfg.SaluteSpeechAuthorizationKey)
	}
	if cfg.SaluteSpeechScope != "SALUTE_SPEECH_PERS" {
		t.Errorf("SaluteSpeechScope: want %q, got %q", "SALUTE_SPEECH_PERS", cfg.SaluteSpeechScope)
	}
}

func TestLoadFlags_IgnoresZeroValues(t *testing.T) {
	cfg := &Config{TelegramToken: "original", LogLevel: "info", WorkerCount: 5, QueueSize: 100}
	loadFlags(cfg, flagValues{})

	if cfg.TelegramToken != "original" {
		t.Errorf("TelegramToken should be unchanged, got %q", cfg.TelegramToken)
	}
	if cfg.WorkerCount != 5 {
		t.Errorf("WorkerCount should be unchanged, got %d", cfg.WorkerCount)
	}
	if cfg.DatabaseDSN != "" {
		t.Errorf("DatabaseDSN should be unchanged, got %q", cfg.DatabaseDSN)
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
