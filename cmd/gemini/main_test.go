package main

import (
	"os"
	"testing"

	"github.com/ai8future/chassis-go"
	chassisconfig "github.com/ai8future/chassis-go/config"
	"github.com/ai8future/chassis-go/testkit"
)

func TestMain(m *testing.M) {
	chassis.RequireMajor(4)
	os.Exit(m.Run())
}

func TestConfig_Defaults(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"GEMINI_API_KEY": "test-key-123",
	})

	cfg := chassisconfig.MustLoad[Config]()

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
	if cfg.GoogleSearch != true {
		t.Errorf("GoogleSearch: got %v, want true", cfg.GoogleSearch)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "error")
	}
}

func TestConfig_Overrides(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"GEMINI_API_KEY":          "key-override",
		"GEMINI_MODEL":            "gemini-2.0-flash",
		"GEMINI_MAX_TOKENS":       "8192",
		"GEMINI_TEMPERATURE":      "0.5",
		"GEMINI_TIMEOUT":          "10s",
		"GEMINI_GOOGLE_SEARCH":    "false",
		"LOG_LEVEL":               "debug",
	})

	cfg := chassisconfig.MustLoad[Config]()

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
	if cfg.GoogleSearch != false {
		t.Errorf("GoogleSearch: got %v, want false", cfg.GoogleSearch)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestConfig_PanicsWithoutAPIKey(t *testing.T) {
	testkit.SetEnv(t, map[string]string{})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing GEMINI_API_KEY")
		}
	}()
	_ = chassisconfig.MustLoad[Config]()
}
