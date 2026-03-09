package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ai8future/chassis-go-addons/llm"
)

const (
	defaultBaseURL    = "https://generativelanguage.googleapis.com/v1beta/models"
	defaultModel      = "gemini-3-pro-preview"
	defaultTimeout    = 30 * time.Second
	maxResponseBytes  = 10 * 1024 * 1024 // 10 MB
	maxErrorBodyBytes = 1024             // truncate error bodies in messages
	maxTemperature    = 2.0
	maxMaxTokens      = 1_000_000
)

// validModel matches model names: alphanumeric, dots, hyphens, underscores, slashes.
var validModel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// Doer executes HTTP requests. Satisfied by *http.Client, call.Client, and test mocks.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client is a Gemini API client.
type Client struct {
	apiKey    string
	model     string
	baseURL   string
	doer      Doer
	llmClient *llm.Client // addon client for standard (non-tool) calls
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

// WithTimeout sets the timeout on the default HTTP client.
// Ignored when WithDoer is also used, since the caller controls their own client.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if hc, ok := c.doer.(*http.Client); ok {
			hc.Timeout = d
		}
	}
}

// New creates a Gemini client with the given API key and options.
func New(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("gemini: API key must not be empty")
	}
	c := &Client{
		apiKey:  apiKey,
		model:   defaultModel,
		baseURL: defaultBaseURL,
		doer:    &http.Client{Timeout: defaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	if !strings.HasPrefix(c.baseURL, "https://") {
		return nil, fmt.Errorf("gemini: base URL must use HTTPS, got %q", c.baseURL)
	}
	if c.model == "" {
		return nil, errors.New("gemini: model must not be empty")
	}
	if !validModel.MatchString(c.model) {
		return nil, fmt.Errorf("gemini: invalid model name %q", c.model)
	}

	// Build an llm addon client when the doer is a standard *http.Client.
	// This covers normal production use. When a custom Doer (e.g. a test
	// mock) is injected, the addon path is skipped and doRequest is used
	// as a fallback.
	if hc, ok := c.doer.(*http.Client); ok {
		addonClient, err := llm.NewClient(llm.Options{
			Provider: llm.Gemini,
			APIKey:   apiKey,
			Model:    c.model,
			BaseURL:  addonBaseURL(c.baseURL, c.model),
			Doer:     hc,
		})
		if err == nil {
			c.llmClient = addonClient
		}
		// If addon creation fails (shouldn't normally), fall back silently
		// to doRequest.
	}

	return c, nil
}

// addonBaseURL converts the gemini module's per-model baseURL (which already
// includes "/models") into the base URL the addon expects (without "/models").
// The addon constructs URLs as: baseURL + "/models/" + model + ":generateContent"
// Our baseURL already looks like ".../v1beta/models", so we strip the trailing
// "/models" to let the addon reconstruct it.
func addonBaseURL(baseURL, _ string) string {
	if strings.HasSuffix(baseURL, "/models") {
		return strings.TrimSuffix(baseURL, "/models")
	}
	// Non-standard base URL — strip trailing model path segments so the
	// addon can reconstruct. If the URL doesn't end with "/models", pass
	// it through as-is (the addon's URL construction may differ, so we'll
	// fall through to doRequest if the addon wasn't created).
	return baseURL
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

// Generate sends a prompt to the Gemini API and returns the parsed response.
func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
	cfg := &generateConfig{
		maxTokens:   32000,
		temperature: 1.0,
	}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.maxTokens <= 0 || cfg.maxTokens > maxMaxTokens {
		return nil, fmt.Errorf("gemini: maxTokens must be between 1 and %d, got %d", maxMaxTokens, cfg.maxTokens)
	}
	if cfg.temperature < 0 || cfg.temperature > maxTemperature {
		return nil, fmt.Errorf("gemini: temperature must be between 0 and %.1f, got %f", maxTemperature, cfg.temperature)
	}

	// Use the llm addon for standard calls (no GoogleSearch, addon available).
	if !cfg.googleSearch && c.llmClient != nil {
		return c.generateViaAddon(ctx, prompt, cfg)
	}

	// Fall back to hand-rolled HTTP for GoogleSearch calls or when addon
	// is not available (e.g. custom Doer injected in tests).
	return c.generateViaHTTP(ctx, prompt, cfg)
}

// generateViaAddon uses the chassis-go-addons/llm module.
func (c *Client) generateViaAddon(ctx context.Context, prompt string, cfg *generateConfig) (*Response, error) {
	chatReq := llm.ChatRequest{
		Messages:    []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens:   llm.Int(cfg.maxTokens),
		Temperature: llm.Float64(cfg.temperature),
	}

	chatResp, err := c.llmClient.Chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	// Map the addon response to the exported Response type.
	resp := &Response{
		Candidates: []Candidate{{
			Content: ResponseContent{
				Parts: []ResponsePart{{Text: chatResp.Content}},
				Role:  "model",
			},
			FinishReason: "STOP",
		}},
		UsageMetadata: UsageMetadata{
			PromptTokenCount:     chatResp.Usage.InputTokens,
			CandidatesTokenCount: chatResp.Usage.OutputTokens,
			TotalTokenCount:      chatResp.Usage.TotalTokens,
		},
	}
	return resp, nil
}

// generateViaHTTP is the original hand-rolled HTTP path. Used for GoogleSearch
// calls (the llm addon does not support Tools) and when a custom Doer is injected.
func (c *Client) generateViaHTTP(ctx context.Context, prompt string, cfg *generateConfig) (*Response, error) {
	reqBody := Request{
		Contents: []Content{
			{Role: "user", Parts: []Part{{Text: prompt}}},
		},
		GenerationConfig: GenerationConfig{
			MaxOutputTokens: cfg.maxTokens,
			Temperature:     &cfg.temperature,
		},
	}

	if cfg.googleSearch {
		reqBody.Tools = []Tool{{GoogleSearch: &GoogleSearch{}}}
	}

	var resp Response
	if err := c.doRequest(ctx, &reqBody, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// doRequest performs an HTTP request to the Gemini API.
func (c *Client) doRequest(ctx context.Context, reqBody, respBody any) error {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent", c.baseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("gemini: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	// Allow retry middleware to replay the body on subsequent attempts.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(jsonData)), nil
	}

	resp, err := c.doer.Do(req)
	if err != nil {
		return fmt.Errorf("gemini: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("gemini: read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("gemini: response exceeds %d byte limit", maxResponseBytes)
	}

	if resp.StatusCode >= 400 {
		msg := string(body)
		if len(msg) > maxErrorBodyBytes {
			msg = msg[:maxErrorBodyBytes] + "...(truncated)"
		}
		return fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, msg)
	}

	if err := json.Unmarshal(body, respBody); err != nil {
		return fmt.Errorf("gemini: unmarshal response: %w", err)
	}

	return nil
}
