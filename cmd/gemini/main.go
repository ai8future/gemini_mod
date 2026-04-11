// Package main provides a CLI tool for the Gemini generative AI API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	chassis "github.com/ai8future/chassis-go/v11"
	"github.com/ai8future/chassis-go/v11/call"
	chassisconfig "github.com/ai8future/chassis-go/v11/config"
	"github.com/ai8future/chassis-go/v11/deploy"
	"github.com/ai8future/chassis-go/v11/logz"
	"github.com/ai8future/chassis-go/v11/registry"

	aigeminimod "ai_gemini_mod"
	"ai_gemini_mod/gemini"
)


const (
	retryAttempts    = 3
	retryBaseDelay   = 500 * time.Millisecond
	retryTotalAttempts = retryAttempts + 1 // initial attempt + retries
)

// Config holds CLI configuration loaded from environment.
type Config struct {
	APIKey       string        `env:"GEMINI_API_KEY" required:"true"`
	Model        string        `env:"GEMINI_MODEL" default:"gemini-3-pro-preview"`
	MaxTokens    int           `env:"GEMINI_MAX_TOKENS" default:"32000"`
	Temperature  float64       `env:"GEMINI_TEMPERATURE" default:"1.0"`
	Timeout      time.Duration `env:"GEMINI_TIMEOUT" default:"30s"`
	GoogleSearch bool          `env:"GEMINI_GOOGLE_SEARCH" default:"true"`
	LogLevel     string        `env:"LOG_LEVEL" default:"error"`
}

func main() {
	chassis.SetAppVersion(aigeminimod.AppVersion)
	chassis.RequireMajor(11)

	d := deploy.Discover("ai_gemini_mod")
	d.LoadEnv()

	if err := registry.InitCLI(chassis.Version); err != nil {
		log.Fatalf("registry: %v", err)
	}

	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		registry.ShutdownCLI(1)
		os.Exit(1)
	}

	registry.ShutdownCLI(0)
}

func run(args []string) error {
	cfg := chassisconfig.MustLoad[Config]()
	logger := logz.New(cfg.LogLevel)
	logger.Info("starting", "chassis", chassis.Version)

	if len(args) == 0 {
		return fmt.Errorf("usage: gemini <prompt>")
	}
	prompt := strings.Join(args, " ")

	logger.Debug("request config", "model", cfg.Model, "max_tokens", cfg.MaxTokens, "temperature", cfg.Temperature)

	caller := call.New(
		call.WithTimeout(cfg.Timeout),
		call.WithRetry(retryAttempts, retryBaseDelay),
	)

	client, err := gemini.New(cfg.APIKey,
		gemini.WithModel(cfg.Model),
		gemini.WithDoer(caller),
	)
	if err != nil {
		return err
	}

	// Allow enough time for all retry attempts; +1 provides buffer for backoff gaps.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalAttempts+1))
	defer cancel()

	genOpts := []gemini.GenerateOption{
		gemini.WithMaxTokens(cfg.MaxTokens),
		gemini.WithTemperature(cfg.Temperature),
	}
	if cfg.GoogleSearch {
		genOpts = append(genOpts, gemini.WithGoogleSearch())
	}

	resp, err := client.Generate(ctx, prompt, genOpts...)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting response: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
