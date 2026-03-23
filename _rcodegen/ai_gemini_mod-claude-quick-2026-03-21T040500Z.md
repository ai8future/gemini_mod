Date Created: 2026-03-21T04:05:00Z
TOTAL_SCORE: 90/100

# ai_gemini_mod — Quick Analysis Report

**Agent:** Claude:Opus 4.6
**Scope:** gemini/ package (client.go, types.go), cmd/gemini/ (main.go), and all test files
**Files analyzed:** 5 Go source files (~1,211 lines total)

---

## 1. AUDIT — Security and Code Quality Issues

### A-1: Error from addon path leaks implementation details (Severity: Medium)

`generateViaAddon` returns the raw error from the addon client without wrapping it with the `"gemini:"` prefix used consistently everywhere else. This inconsistency means callers cannot reliably match error prefixes, and internal addon errors may leak implementation details.

**File:** `gemini/client.go:196-198`

```diff
 func (c *Client) generateViaAddon(ctx context.Context, prompt string, cfg *generateConfig) (*Response, error) {
 	chatReq := llm.ChatRequest{
 		Messages:    []llm.Message{{Role: "user", Content: prompt}},
 		MaxTokens:   llm.Int(cfg.maxTokens),
 		Temperature: llm.Float64(cfg.temperature),
 	}

 	chatResp, err := c.llmClient.Chat(ctx, chatReq)
 	if err != nil {
-		return nil, err
+		return nil, fmt.Errorf("gemini: generate: %w", err)
 	}
```

### A-2: No prompt size validation (Severity: Low)

The `Generate` method validates `maxTokens` and `temperature` but does not validate the prompt string. An empty or excessively large prompt would be sent to the API without any client-side guard. While the API will reject bad prompts, a multi-MB prompt would be serialized, sent over the wire, and consume bandwidth before being rejected.

**File:** `gemini/client.go:161-175`

```diff
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
+	if strings.TrimSpace(prompt) == "" {
+		return nil, errors.New("gemini: prompt must not be empty")
+	}
+
 	cfg := &generateConfig{
 		maxTokens:   32000,
 		temperature: 1.0,
 	}
```

### A-3: Silent swallow of addon creation error (Severity: Low)

When the addon client fails to initialize, the error is silently discarded. In production, this could mask misconfiguration (e.g., invalid base URL) with no log output. Consider at minimum logging the error at debug level.

**File:** `gemini/client.go:110-114`

```diff
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
-		// If addon creation fails (shouldn't normally), fall back silently
-		// to doRequest.
+		// If addon creation fails, fall back to doRequest. This should
+		// not happen in normal use; callers can detect the fallback by
+		// checking whether llmClient is nil after construction.
 	}
```

### A-4: API key stored as plain string in struct (Severity: Informational)

The API key is stored as a plain `string` in the `Client` struct. This is standard Go practice but means the key is visible in heap dumps, core files, and debug output. No action required for this codebase size, but worth noting for compliance audits.

---

## 2. TESTS — Proposed Unit Tests

### T-1: Test `addonBaseURL` helper function

This function has two branches but zero tests.

**File:** `gemini/client_test.go` (append)

```diff
+func TestAddonBaseURL(t *testing.T) {
+	tests := []struct {
+		name    string
+		baseURL string
+		want    string
+	}{
+		{
+			name:    "standard URL with /models suffix",
+			baseURL: "https://generativelanguage.googleapis.com/v1beta/models",
+			want:    "https://generativelanguage.googleapis.com/v1beta",
+		},
+		{
+			name:    "custom URL without /models suffix",
+			baseURL: "https://custom.api.example.com/v1",
+			want:    "https://custom.api.example.com/v1",
+		},
+		{
+			name:    "URL ending with /models/ (trailing slash already stripped)",
+			baseURL: "https://example.com/v1/models",
+			want:    "https://example.com/v1",
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := addonBaseURL(tt.baseURL, "ignored-model")
+			if got != tt.want {
+				t.Errorf("addonBaseURL(%q): got %q, want %q", tt.baseURL, got, tt.want)
+			}
+		})
+	}
+}
```

### T-2: Test empty prompt handling

No test verifies behavior when an empty string is passed as the prompt. (If A-2 is applied, this tests the new validation; if not, it documents the current pass-through behavior.)

**File:** `gemini/client_test.go` (append)

```diff
+func TestGenerate_EmptyPrompt(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	// Current behavior: empty prompt is sent to API.
+	// If prompt validation is added, this should return an error.
+	_, err := c.Generate(context.Background(), "")
+	if err != nil {
+		// Prompt validation was added — verify the error message.
+		if !strings.Contains(err.Error(), "prompt must not be empty") {
+			t.Errorf("unexpected error: %v", err)
+		}
+		return
+	}
+
+	// No validation — verify the empty prompt was sent through.
+	var req Request
+	if err := json.Unmarshal(mock.body, &req); err != nil {
+		t.Fatalf("unmarshal: %v", err)
+	}
+	if req.Contents[0].Parts[0].Text != "" {
+		t.Errorf("expected empty prompt, got %q", req.Contents[0].Parts[0].Text)
+	}
+}
```

### T-3: Test Generate with cancelled context

Ensures context cancellation is handled before the HTTP call executes.

**File:** `gemini/client_test.go` (append)

```diff
+func TestGenerate_CancelledContext(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel() // cancel immediately
+
+	_, err := c.Generate(ctx, "test")
+	if err == nil {
+		t.Fatal("expected error for cancelled context")
+	}
+	if !strings.Contains(err.Error(), "context canceled") {
+		t.Errorf("expected context canceled error, got: %v", err)
+	}
+}
```

### T-4: Test HTTP 200 with valid but empty JSON object

Ensures the client handles a 200 response with `{}` (no candidates) gracefully.

**File:** `gemini/client_test.go` (append)

```diff
+func TestGenerate_EmptySuccessResponse(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	resp, err := c.Generate(context.Background(), "test")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	if resp.Text() != "" {
+		t.Errorf("expected empty text for response with no candidates, got %q", resp.Text())
+	}
+	if len(resp.Candidates) != 0 {
+		t.Errorf("expected 0 candidates, got %d", len(resp.Candidates))
+	}
+}
```

### T-5: Test `run()` with no arguments

**File:** `cmd/gemini/main_test.go` (append)

```diff
+func TestRun_NoArgs(t *testing.T) {
+	testkit.SetEnv(t, map[string]string{
+		"GEMINI_API_KEY": "test-key",
+	})
+
+	err := run([]string{})
+	if err == nil {
+		t.Fatal("expected error for no arguments")
+	}
+	if !strings.Contains(err.Error(), "usage:") {
+		t.Errorf("expected usage error, got: %v", err)
+	}
+}
```

### T-6: Test boundary value — maxTokens at exactly 1,000,000

**File:** `gemini/client_test.go` (append)

```diff
+func TestGenerate_MaxTokensBoundary(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	// Exactly at upper bound — should succeed.
+	_, err := c.Generate(context.Background(), "test", WithMaxTokens(1_000_000))
+	if err != nil {
+		t.Fatalf("maxTokens=1_000_000 should be valid, got error: %v", err)
+	}
+
+	// One over — should fail.
+	_, err = c.Generate(context.Background(), "test", WithMaxTokens(1_000_001))
+	if err == nil {
+		t.Fatal("expected error for maxTokens=1_000_001")
+	}
+}
```

### T-7: Test boundary value — temperature at exactly 2.0

**File:** `gemini/client_test.go` (append)

```diff
+func TestGenerate_TemperatureBoundary(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	// Exactly at upper bound — should succeed.
+	_, err := c.Generate(context.Background(), "test", WithTemperature(2.0))
+	if err != nil {
+		t.Fatalf("temperature=2.0 should be valid, got error: %v", err)
+	}
+
+	// Slightly over — should fail.
+	_, err = c.Generate(context.Background(), "test", WithTemperature(2.01))
+	if err == nil {
+		t.Fatal("expected error for temperature=2.01")
+	}
+}
```

---

## 3. FIXES — Bugs, Issues, and Code Smells

### F-1: Duplicated default values risk drift (Severity: Low)

`generateConfig` hardcodes `maxTokens: 32000` and `temperature: 1.0` at `client.go:163-164`, while `Config` in `main.go:33-34` specifies the same values via struct tags. If either changes independently, behavior diverges silently between library and CLI usage.

**File:** `gemini/client.go:162-165`

```diff
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
 	cfg := &generateConfig{
-		maxTokens:   32000,
-		temperature: 1.0,
+		maxTokens:   DefaultMaxTokens,
+		temperature: DefaultTemperature,
 	}
```

And add exported constants:

**File:** `gemini/client.go` (add after existing constants block)

```diff
+	// DefaultMaxTokens is the default maximum output tokens per request.
+	DefaultMaxTokens = 32_000
+	// DefaultTemperature is the default sampling temperature.
+	DefaultTemperature = 1.0
```

Then update `cmd/gemini/main.go` to reference them:

```diff
 type Config struct {
 	APIKey       string        `env:"GEMINI_API_KEY" required:"true"`
-	Model        string        `env:"GEMINI_MODEL" default:"gemini-3-pro-preview"`
-	MaxTokens    int           `env:"GEMINI_MAX_TOKENS" default:"32000"`
-	Temperature  float64       `env:"GEMINI_TEMPERATURE" default:"1.0"`
+	Model        string        `env:"GEMINI_MODEL" default:"gemini-3-pro-preview"` // matches gemini.defaultModel
+	MaxTokens    int           `env:"GEMINI_MAX_TOKENS" default:"32000"`           // matches gemini.DefaultMaxTokens
+	Temperature  float64       `env:"GEMINI_TEMPERATURE" default:"1.0"`            // matches gemini.DefaultTemperature
```

*Note: The struct tag `default:` values can't reference Go constants, so comments are the pragmatic solution. The real fix is the exported constants so library consumers don't have to guess the defaults.*

### F-2: Unused parameter in `addonBaseURL` (Severity: Low)

The second parameter (model) was likely used during development but is now `_`. Clean dead parameter.

**File:** `gemini/client.go:125`

```diff
-func addonBaseURL(baseURL, _ string) string {
+func addonBaseURL(baseURL string) string {
```

And the call site at line 107:

```diff
-			BaseURL:  addonBaseURL(c.baseURL, c.model),
+			BaseURL:  addonBaseURL(c.baseURL),
```

### F-3: HTTP status check should distinguish client vs. server errors (Severity: Informational)

Currently all HTTP 400+ errors produce the same format: `"gemini: HTTP %d: %s"`. Distinguishing 4xx (client error — don't retry) from 5xx (server error — may retry) in the error message would help with debugging and retry logic.

**File:** `gemini/client.go:277-283`

```diff
 	if resp.StatusCode >= 400 {
 		msg := string(body)
 		if len(msg) > maxErrorBodyBytes {
 			msg = msg[:maxErrorBodyBytes] + "...(truncated)"
 		}
-		return fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, msg)
+		kind := "server error"
+		if resp.StatusCode < 500 {
+			kind = "client error"
+		}
+		return fmt.Errorf("gemini: HTTP %d (%s): %s", resp.StatusCode, kind, msg)
 	}
```

*Note: This would require updating test assertions that match on `"HTTP 429"` etc. The existing format is functional — this is a nice-to-have.*

---

## 4. REFACTOR — Improvement Opportunities (No Diffs)

### R-1: Extract response mapping into a shared helper

Both `generateViaAddon` and `generateViaHTTP` construct `*Response` but through different paths. If additional response post-processing is added (e.g., safety filtering, token budget tracking), it would need to be duplicated. A small `mapResponse` or post-processing hook would centralize this.

### R-2: Consider a `ClientInfo()` method for debugging

A method that returns the configured model, base URL, and whether the addon path is active would help operators diagnose routing issues without exposing the API key. Example: `client.Info() → {Model: "gemini-3-pro-preview", BaseURL: "...", AddonEnabled: true}`.

### R-3: Package-level documentation

`types.go` has a package doc comment but `client.go` does not. Go convention is one package doc across the package. The existing one in `types.go` is minimal — a slightly expanded version explaining the dual-path architecture would help new contributors.

### R-4: Consider structured errors

The codebase uses `fmt.Errorf` consistently, which is clean but makes programmatic error handling harder. Introducing a `gemini.Error` type with fields for HTTP status, kind (client/server), and body would allow callers to switch on error type rather than string matching. This is overkill for the current codebase size but would be valuable if the API surface grows.

### R-5: Test table consolidation

Several related tests (e.g., `TestGenerate_NegativeMaxTokens`, `TestGenerate_ZeroMaxTokens`, `TestGenerate_MaxTokensTooHigh`) could be consolidated into a single table-driven test like `TestNew_InvalidModelName` already does. This would reduce boilerplate and make it easier to add new boundary cases.

---

## Score Breakdown

| Category | Score | Notes |
|----------|-------|-------|
| **Security** | 23/25 | Excellent. HTTPS enforcement, API key in header, response size limits, model regex. Minor: no prompt validation, silent addon failure. |
| **Code Quality** | 23/25 | Clean idiomatic Go, functional options, consistent error wrapping. Minor: addon error not wrapped, unused param, duplicated defaults. |
| **Test Coverage** | 20/25 | 59+ test cases covering happy paths, edge cases, and error conditions. Gap: addon path untested, no `addonBaseURL` tests, no CLI `run()` integration test. |
| **Architecture** | 24/25 | Clean separation, good DI, retry support, dual HTTP paths. Very minor: could benefit from structured errors as API grows. |
| **TOTAL** | **90/100** | A well-engineered, production-quality Go module with strong security posture and comprehensive testing. |
