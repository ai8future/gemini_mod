package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mockDoer captures the request and returns a canned response.
type mockDoer struct {
	req        *http.Request
	body       []byte
	statusCode int
	respBody   string
	err        error
}

func (m *mockDoer) Do(req *http.Request) (*http.Response, error) {
	m.req = req
	if req.Body != nil {
		m.body, _ = io.ReadAll(req.Body)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.respBody)),
	}, nil
}

func TestNew_Defaults(t *testing.T) {
	c, err := New("test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.apiKey != "test-key" {
		t.Errorf("apiKey: got %q, want %q", c.apiKey, "test-key")
	}
	if c.model != defaultModel {
		t.Errorf("model: got %q, want %q", c.model, defaultModel)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL: got %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.doer == nil {
		t.Error("doer should not be nil")
	}
	httpClient, ok := c.doer.(*http.Client)
	if !ok {
		t.Fatal("doer should be an *http.Client")
	}
	if httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout: got %v, want %v", httpClient.Timeout, defaultTimeout)
	}
}

func TestNew_EmptyAPIKey(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
	if !strings.Contains(err.Error(), "API key must not be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNew_HTTPBaseURL(t *testing.T) {
	_, err := New("key", WithBaseURL("http://insecure.example.com"))
	if err == nil {
		t.Fatal("expected error for HTTP base URL")
	}
	if !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNew_WithOptions(t *testing.T) {
	mock := &mockDoer{}
	c, err := New("key",
		WithModel("custom-model"),
		WithDoer(mock),
		WithBaseURL("https://example.com/api"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.model != "custom-model" {
		t.Errorf("model: got %q, want %q", c.model, "custom-model")
	}
	if c.baseURL != "https://example.com/api" {
		t.Errorf("baseURL: got %q, want %q", c.baseURL, "https://example.com/api")
	}
}

func mustNew(t *testing.T, apiKey string, opts ...Option) *Client {
	t.Helper()
	c, err := New(apiKey, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestGenerate_RequestBuilding(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "my-api-key", WithDoer(mock), WithModel("test-model"), WithBaseURL("https://api.test"))

	_, _ = c.Generate(context.Background(), "hello world",
		WithMaxTokens(100),
		WithTemperature(0.5),
		WithGoogleSearch(),
	)

	if mock.req == nil {
		t.Fatal("expected request to be captured")
	}

	// Verify URL.
	wantURL := "https://api.test/test-model:generateContent"
	if mock.req.URL.String() != wantURL {
		t.Errorf("URL: got %q, want %q", mock.req.URL.String(), wantURL)
	}

	// Verify headers.
	if got := mock.req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", got, "application/json")
	}
	if got := mock.req.Header.Get("x-goog-api-key"); got != "my-api-key" {
		t.Errorf("x-goog-api-key: got %q, want %q", got, "my-api-key")
	}

	// Verify body.
	var req Request
	if err := json.Unmarshal(mock.body, &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if req.Contents[0].Parts[0].Text != "hello world" {
		t.Errorf("prompt: got %q, want %q", req.Contents[0].Parts[0].Text, "hello world")
	}
	if req.GenerationConfig.MaxOutputTokens != 100 {
		t.Errorf("maxOutputTokens: got %d, want 100", req.GenerationConfig.MaxOutputTokens)
	}
	if req.GenerationConfig.Temperature == nil || *req.GenerationConfig.Temperature != 0.5 {
		t.Errorf("temperature: got %v, want 0.5", req.GenerationConfig.Temperature)
	}
	if len(req.Tools) != 1 || req.Tools[0].GoogleSearch == nil {
		t.Error("expected googleSearch tool")
	}
}

func TestGenerate_NoGoogleSearch(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))

	_, _ = c.Generate(context.Background(), "test")

	var req Request
	if err := json.Unmarshal(mock.body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.Tools) != 0 {
		t.Errorf("expected no tools, got %d", len(req.Tools))
	}
}

func TestGenerate_Success(t *testing.T) {
	respJSON := `{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello back!"}],
				"role": "model"
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 5,
			"candidatesTokenCount": 10,
			"totalTokenCount": 15
		}
	}`
	mock := &mockDoer{statusCode: 200, respBody: respJSON}
	c := mustNew(t, "key", WithDoer(mock))

	resp, err := c.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.Text(); got != "Hello back!" {
		t.Errorf("Text(): got %q, want %q", got, "Hello back!")
	}
	if resp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("TotalTokenCount: got %d, want 15", resp.UsageMetadata.TotalTokenCount)
	}
}

func TestGenerate_HTTPError(t *testing.T) {
	mock := &mockDoer{statusCode: 429, respBody: `{"error":"rate limited"}`}
	c := mustNew(t, "key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
	if !strings.Contains(err.Error(), "HTTP 429") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

func TestGenerate_DoerError(t *testing.T) {
	mock := &mockDoer{err: io.ErrUnexpectedEOF}
	c := mustNew(t, "key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error from doer")
	}
	if !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("error should propagate doer error, got: %v", err)
	}
}

func TestResponse_TextNoCandidates(t *testing.T) {
	r := &Response{}
	if got := r.Text(); got != "" {
		t.Errorf("Text(): got %q, want empty", got)
	}
}

func TestResponse_TextNoParts(t *testing.T) {
	r := &Response{
		Candidates: []Candidate{{Content: ResponseContent{Parts: nil}}},
	}
	if got := r.Text(); got != "" {
		t.Errorf("Text(): got %q, want empty", got)
	}
}

func TestResponse_TextMultipleParts(t *testing.T) {
	r := &Response{
		Candidates: []Candidate{{
			Content: ResponseContent{
				Parts: []ResponsePart{
					{Text: "Hello "},
					{Text: "world!"},
				},
			},
		}},
	}
	if got := r.Text(); got != "Hello world!" {
		t.Errorf("Text(): got %q, want %q", got, "Hello world!")
	}
}

func TestGenerate_NegativeMaxTokens(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))
	_, err := c.Generate(context.Background(), "test", WithMaxTokens(-1))
	if err == nil {
		t.Fatal("expected error for negative maxTokens")
	}
}

func TestGenerate_NegativeTemperature(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))
	_, err := c.Generate(context.Background(), "test", WithTemperature(-0.5))
	if err == nil {
		t.Fatal("expected error for negative temperature")
	}
}

func TestGenerate_ContextPropagated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))

	_, _ = c.Generate(ctx, "test")

	if mock.req.Context() != ctx {
		t.Error("expected request to carry the provided context")
	}
}

func TestGenerate_GetBody(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))

	_, _ = c.Generate(context.Background(), "test")

	if mock.req == nil {
		t.Fatal("expected request to be captured")
	}
	if mock.req.GetBody == nil {
		t.Fatal("expected GetBody to be set for retry support")
	}

	body, err := mock.req.GetBody()
	if err != nil {
		t.Fatalf("GetBody error: %v", err)
	}
	data, _ := io.ReadAll(body)
	if len(data) == 0 {
		t.Error("GetBody returned empty body")
	}

	// Verify the replayed body matches the original.
	var reqBody Request
	if err := json.Unmarshal(data, &reqBody); err != nil {
		t.Fatalf("unmarshal replayed body: %v", err)
	}
	if reqBody.Contents[0].Parts[0].Text != "test" {
		t.Errorf("replayed prompt: got %q, want %q", reqBody.Contents[0].Parts[0].Text, "test")
	}
}

// --- Validation edge cases ---

func TestNew_WhitespaceOnlyAPIKey(t *testing.T) {
	_, err := New("   \t\n")
	if err == nil {
		t.Fatal("expected error for whitespace-only API key")
	}
}

func TestGenerate_ZeroMaxTokens(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))
	_, err := c.Generate(context.Background(), "test", WithMaxTokens(0))
	if err == nil {
		t.Fatal("expected error for zero maxTokens")
	}
}

func TestGenerate_ZeroTemperature(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))
	resp, err := c.Generate(context.Background(), "test", WithTemperature(0.0))
	if err != nil {
		t.Fatalf("temperature 0.0 should be valid, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestNew_EmptyModel(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock), WithModel(""))
	_, _ = c.Generate(context.Background(), "test")

	// Empty model produces URL like "https://…/:generateContent"
	got := mock.req.URL.String()
	if !strings.Contains(got, "/:generateContent") {
		t.Errorf("expected empty model segment in URL, got %q", got)
	}
}

// --- Response parsing edge cases ---

func TestGenerate_InvalidJSONResponse(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{not valid json`}
	c := mustNew(t, "key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "unmarshal response") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestGenerate_EmptyResponseBody(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: ""}
	c := mustNew(t, "key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty response body")
	}
	if !strings.Contains(err.Error(), "unmarshal response") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestGenerate_ResponseExceedsMaxBytes(t *testing.T) {
	// Build a response body larger than maxResponseBytes (10 MB).
	bigBody := strings.Repeat("x", maxResponseBytes+100)
	mock := &mockDoer{statusCode: 200, respBody: bigBody}
	c := mustNew(t, "key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size limit error, got: %v", err)
	}
}

func TestGenerate_ErrorBodyTruncation(t *testing.T) {
	// Build an error body longer than maxErrorBodyBytes (1024).
	longError := strings.Repeat("E", 2000)
	mock := &mockDoer{statusCode: 500, respBody: longError}
	c := mustNew(t, "key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "...(truncated)") {
		t.Errorf("expected truncation marker in error, got: %v", err)
	}
	// The full 2000-char body should not appear in the error.
	if strings.Contains(errMsg, longError) {
		t.Error("error message should not contain full error body")
	}
}

// --- Serialization tests ---

func TestGenerationConfig_OmitemptyJSON(t *testing.T) {
	// Zero-value GenerationConfig should produce minimal JSON.
	cfg := GenerationConfig{}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	if got != "{}" {
		t.Errorf("zero GenerationConfig should marshal to {}, got %s", got)
	}
}

func TestGenerationConfig_TemperatureZeroIncluded(t *testing.T) {
	// When temperature pointer is set to 0.0, it should be included in JSON.
	temp := 0.0
	cfg := GenerationConfig{Temperature: &temp}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"temperature":0`) {
		t.Errorf("expected temperature:0 in JSON, got %s", got)
	}
}

func TestGenerate_TemperatureZeroInRequest(t *testing.T) {
	// Verify that WithTemperature(0.0) serializes temperature in the request body.
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock))

	_, _ = c.Generate(context.Background(), "test", WithTemperature(0.0))

	bodyStr := string(mock.body)
	if !strings.Contains(bodyStr, `"temperature":0`) {
		t.Errorf("expected temperature:0 in request body, got %s", bodyStr)
	}
}

// --- URL construction ---

func TestGenerate_ModelWithSpecialChars(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := mustNew(t, "key", WithDoer(mock), WithModel("models/gemini-2.0-flash"))

	_, _ = c.Generate(context.Background(), "test")

	got := mock.req.URL.String()
	wantSuffix := "models/gemini-2.0-flash:generateContent"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("URL should end with %q, got %q", wantSuffix, got)
	}
}

// --- Multi-candidate ---

func TestResponse_TextMultipleCandidates(t *testing.T) {
	r := &Response{
		Candidates: []Candidate{
			{Content: ResponseContent{Parts: []ResponsePart{{Text: "first"}}}},
			{Content: ResponseContent{Parts: []ResponsePart{{Text: "second"}}}},
		},
	}
	if got := r.Text(); got != "first" {
		t.Errorf("Text() should return first candidate, got %q", got)
	}
}
