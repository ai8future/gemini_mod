package main

import (
	"testing"

	"github.com/ai8future/chassis-go/config"
	"github.com/ai8future/chassis-go/testkit"
)

func TestConfigLoading(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"GEMINI_API_KEY": "test-key-123",
	})

	cfg := config.MustLoad[Config]()

	if cfg.APIKey != "test-key-123" {
		t.Errorf("APIKey: got %q, want %q", cfg.APIKey, "test-key-123")
	}
	if cfg.Model != "gemini-3-pro-preview" {
		t.Errorf("Model: got %q, want %q", cfg.Model, "gemini-3-pro-preview")
	}
	if cfg.MaxTokens != 32000 {
		t.Errorf("MaxTokens: got %d, want %d", cfg.MaxTokens, 32000)
	}
	if cfg.Temperature != 1.0 {
		t.Errorf("Temperature: got %f, want %f", cfg.Temperature, 1.0)
	}
	if cfg.Timeout.String() != "30s" {
		t.Errorf("Timeout: got %s, want 30s", cfg.Timeout)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "error")
	}
}

func TestConfigOverrides(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"GEMINI_API_KEY":     "key-override",
		"GEMINI_MODEL":       "gemini-2.0-flash",
		"GEMINI_MAX_TOKENS":  "8192",
		"GEMINI_TEMPERATURE": "0.5",
		"GEMINI_TIMEOUT":     "10s",
		"LOG_LEVEL":          "debug",
	})

	cfg := config.MustLoad[Config]()

	if cfg.Model != "gemini-2.0-flash" {
		t.Errorf("Model: got %q, want %q", cfg.Model, "gemini-2.0-flash")
	}
	if cfg.MaxTokens != 8192 {
		t.Errorf("MaxTokens: got %d, want %d", cfg.MaxTokens, 8192)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature: got %f, want %f", cfg.Temperature, 0.5)
	}
	if cfg.Timeout.String() != "10s" {
		t.Errorf("Timeout: got %s, want 10s", cfg.Timeout)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestConfigMissingAPIKey(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"GEMINI_API_KEY": "",
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing GEMINI_API_KEY, got none")
		}
	}()

	config.MustLoad[Config]()
}
