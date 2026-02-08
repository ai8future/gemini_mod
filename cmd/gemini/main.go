// Package main provides a CLI tool for the Gemini generative AI API.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ai8future/chassis-go"
	"github.com/ai8future/chassis-go/call"
	chassisconfig "github.com/ai8future/chassis-go/config"
	"github.com/ai8future/chassis-go/logz"

	"ai_gemini_mod/gemini"
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
	chassis.RequireMajor(4)

	cfg := chassisconfig.MustLoad[Config]()
	logger := logz.New(cfg.LogLevel)
	logger.Info("starting", "chassis", chassis.Version)

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gemini <prompt>")
		os.Exit(1)
	}
	prompt := strings.Join(os.Args[1:], " ")

	logger.Debug("request config", "model", cfg.Model, "max_tokens", cfg.MaxTokens, "temperature", cfg.Temperature)

	caller := call.New(
		call.WithTimeout(cfg.Timeout),
		call.WithRetry(3, 500*time.Millisecond),
	)

	client, err := gemini.New(cfg.APIKey,
		gemini.WithModel(cfg.Model),
		gemini.WithDoer(caller),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Allow enough time for all retry attempts (4 total) plus backoff gaps.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*5)
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
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error formatting response: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
