package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
const defaultModel = "gemini-3-pro-preview"

// Doer executes HTTP requests. Satisfied by *http.Client, call.Client, and test mocks.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client is a Gemini API client.
type Client struct {
	apiKey  string
	model   string
	baseURL string
	doer    Doer
}

// Option configures a Client.
type Option func(*Client)

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

// WithDoer sets the HTTP client used for requests.
func WithDoer(d Doer) Option {
	return func(c *Client) { c.doer = d }
}

// WithBaseURL overrides the API base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// New creates a Gemini client with the given API key and options.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		model:   defaultModel,
		baseURL: defaultBaseURL,
		doer:    http.DefaultClient,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// GenerateOption configures a single Generate call.
type GenerateOption func(*generateConfig)

type generateConfig struct {
	maxTokens    int
	temperature  float64
	googleSearch bool
}

// WithMaxTokens sets the max output tokens for a request.
func WithMaxTokens(n int) GenerateOption {
	return func(g *generateConfig) { g.maxTokens = n }
}

// WithTemperature sets the temperature for a request.
func WithTemperature(t float64) GenerateOption {
	return func(g *generateConfig) { g.temperature = t }
}

// WithGoogleSearch enables grounding with Google Search.
func WithGoogleSearch() GenerateOption {
	return func(g *generateConfig) { g.googleSearch = true }
}

// Generate sends a prompt to the Gemini API and returns the raw JSON response bytes.
func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) ([]byte, error) {
	cfg := &generateConfig{
		maxTokens:   32000,
		temperature: 1.0,
	}
	for _, o := range opts {
		o(cfg)
	}

	reqBody := Request{
		Contents: []Content{
			{Parts: []Part{{Text: prompt}}},
		},
		GenerationConfig: GenerationConfig{
			MaxOutputTokens: cfg.maxTokens,
			Temperature:     cfg.temperature,
		},
	}

	if cfg.googleSearch {
		reqBody.Tools = []Tool{{GoogleSearch: &GoogleSearch{}}}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent", c.baseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	// Allow retry middleware to replay the body on subsequent attempts.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(jsonData)), nil
	}

	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, body)
	}

	return body, nil
}
