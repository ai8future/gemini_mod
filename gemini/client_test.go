package gemini

import (
	"bytes"
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

func TestClientDefaults(t *testing.T) {
	c := New("test-key")
	if c.apiKey != "test-key" {
		t.Errorf("apiKey: got %q, want %q", c.apiKey, "test-key")
	}
	if c.model != defaultModel {
		t.Errorf("model: got %q, want %q", c.model, defaultModel)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL: got %q, want %q", c.baseURL, defaultBaseURL)
	}
}

func TestClientOptions(t *testing.T) {
	mock := &mockDoer{}
	c := New("key",
		WithModel("custom-model"),
		WithDoer(mock),
		WithBaseURL("https://example.com/api"),
	)
	if c.model != "custom-model" {
		t.Errorf("model: got %q, want %q", c.model, "custom-model")
	}
	if c.baseURL != "https://example.com/api" {
		t.Errorf("baseURL: got %q, want %q", c.baseURL, "https://example.com/api")
	}
}

func TestGenerateRequestBuilding(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := New("my-api-key", WithDoer(mock), WithModel("test-model"), WithBaseURL("https://api.test"))

	_, _ = c.Generate(context.Background(), "hello world",
		WithMaxTokens(100),
		WithTemperature(0.5),
		WithGoogleSearch(),
	)

	if mock.req == nil {
		t.Fatal("expected request to be captured")
	}

	// Check URL
	wantURL := "https://api.test/test-model:generateContent"
	if mock.req.URL.String() != wantURL {
		t.Errorf("URL: got %q, want %q", mock.req.URL.String(), wantURL)
	}

	// Check headers
	if got := mock.req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", got, "application/json")
	}
	if got := mock.req.Header.Get("x-goog-api-key"); got != "my-api-key" {
		t.Errorf("x-goog-api-key: got %q, want %q", got, "my-api-key")
	}

	// Check body
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
	if req.GenerationConfig.Temperature != 0.5 {
		t.Errorf("temperature: got %f, want 0.5", req.GenerationConfig.Temperature)
	}
	if len(req.Tools) != 1 || req.Tools[0].GoogleSearch == nil {
		t.Error("expected googleSearch tool")
	}
}

func TestGenerateNoGoogleSearch(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := New("key", WithDoer(mock))

	_, _ = c.Generate(context.Background(), "test")

	var req Request
	if err := json.Unmarshal(mock.body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(req.Tools) != 0 {
		t.Errorf("expected no tools, got %d", len(req.Tools))
	}
}

func TestGenerateSuccess(t *testing.T) {
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
	c := New("key", WithDoer(mock))

	body, err := c.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := resp.Text(); got != "Hello back!" {
		t.Errorf("Text(): got %q, want %q", got, "Hello back!")
	}
}

func TestGenerateHTTPError(t *testing.T) {
	mock := &mockDoer{statusCode: 429, respBody: `{"error":"rate limited"}`}
	c := New("key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
	if !strings.Contains(err.Error(), "HTTP 429") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

func TestGenerateDoerError(t *testing.T) {
	mock := &mockDoer{err: io.ErrUnexpectedEOF}
	c := New("key", WithDoer(mock))

	_, err := c.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error from doer")
	}
}

func TestResponseTextNoCandidates(t *testing.T) {
	r := &Response{}
	if got := r.Text(); got != "" {
		t.Errorf("Text(): got %q, want empty", got)
	}
}

func TestResponseTextNoParts(t *testing.T) {
	r := &Response{
		Candidates: []Candidate{{Content: ResponseContent{Parts: nil}}},
	}
	if got := r.Text(); got != "" {
		t.Errorf("Text(): got %q, want empty", got)
	}
}

func TestGenerateContextPropagated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := New("key", WithDoer(mock))

	_, _ = c.Generate(ctx, "test")

	if mock.req.Context() != ctx {
		t.Error("expected request to carry the provided context")
	}
}

func TestGenerateGetBody(t *testing.T) {
	mock := &mockDoer{statusCode: 200, respBody: `{}`}
	c := New("key", WithDoer(mock))

	_, _ = c.Generate(context.Background(), "test")

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
}

func TestResponsePrettyPrint(t *testing.T) {
	respJSON := `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`
	mock := &mockDoer{statusCode: 200, respBody: respJSON}
	c := New("key", WithDoer(mock))

	body, err := c.Generate(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify raw bytes are valid JSON that can be pretty-printed
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		t.Fatalf("failed to pretty-print: %v", err)
	}
}
