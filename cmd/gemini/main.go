package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gemini_mod/gemini"

	"github.com/ai8future/chassis-go/call"
	"github.com/ai8future/chassis-go/config"
	"github.com/ai8future/chassis-go/logz"
)

type Config struct {
	APIKey      string        `env:"GEMINI_API_KEY"`
	Model       string        `env:"GEMINI_MODEL" default:"gemini-3-pro-preview"`
	MaxTokens   int           `env:"GEMINI_MAX_TOKENS" default:"32000"`
	Temperature float64       `env:"GEMINI_TEMPERATURE" default:"1.0"`
	Timeout     time.Duration `env:"GEMINI_TIMEOUT" default:"30s"`
	LogLevel    string        `env:"LOG_LEVEL" default:"error"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: gemini <prompt>")
		os.Exit(1)
	}

	cfg := config.MustLoad[Config]()
	logger := logz.New(cfg.LogLevel)

	prompt := os.Args[1]
	logger.Debug("request config", "model", cfg.Model, "max_tokens", cfg.MaxTokens, "temperature", cfg.Temperature)

	caller := call.New(
		call.WithTimeout(cfg.Timeout),
		call.WithRetry(3, 500*time.Millisecond),
	)

	client := gemini.New(cfg.APIKey,
		gemini.WithModel(cfg.Model),
		gemini.WithDoer(caller),
	)

	body, err := client.Generate(context.Background(), prompt,
		gemini.WithMaxTokens(cfg.MaxTokens),
		gemini.WithTemperature(cfg.Temperature),
		gemini.WithGoogleSearch(),
	)
	if err != nil {
		logger.Error("request failed", "error", err)
		os.Exit(1)
	}

	// Pretty print the JSON response
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Println(string(body))
		return
	}
	fmt.Println(prettyJSON.String())
}
