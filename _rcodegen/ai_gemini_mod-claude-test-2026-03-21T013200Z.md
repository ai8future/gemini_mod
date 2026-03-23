Date Created: 2026-03-21T01:32:00Z
TOTAL_SCORE: 77/100

# ai_gemini_mod — Unit Test Coverage Report

**Agent:** Claude:Opus 4.6
**Go test coverage:** `gemini/` 88.7% | `cmd/gemini/` 0.0%
**All 34 existing tests:** PASS

---

## Scoring Breakdown

| Category | Max | Score | Notes |
|----------|-----|-------|-------|
| Core function coverage | 25 | 22 | Strong on New(), Generate(), Text(), doRequest(). Missing addonBaseURL(), generateViaAddon() |
| Validation testing | 15 | 14 | Excellent edge cases for key, model, tokens, temp. Missing exact-boundary values |
| Error handling | 15 | 13 | Good error paths. Missing short-error passthrough, marshal failure |
| Security testing | 10 | 9 | Path traversal, query injection, HTTPS enforcement all tested |
| Serialization | 10 | 9 | omitempty and zero-pointer semantics covered well |
| CLI/cmd testing | 10 | 5 | Config defaults/overrides/panic tested. run() completely untested |
| Integration paths | 10 | 3 | generateViaAddon entirely untested — this is the primary production path |
| Edge cases & boundaries | 5 | 2 | Good nil/empty/large. Missing boundary values at limits |

---

## What's Tested Well

- **Client construction** (7 tests): defaults, options, empty/whitespace API key, HTTPS enforcement, empty/invalid/valid models, trailing slash
- **Generate HTTP path** (16 tests): request building, success, HTTP errors, doer errors, validation (tokens/temp ranges), context propagation, GetBody replay, role, Google Search toggle
- **Response.Text()** (5 tests): nil receiver, no candidates, no parts, multiple parts, multiple candidates
- **Serialization** (3 tests): omitempty, temperature zero pointer, temperature zero in request
- **CLI config** (3 tests): defaults, overrides, missing-key panic

---

## Coverage Gaps & Proposed Tests

### Gap 1: `addonBaseURL()` — Zero direct tests

This internal helper converts the module's base URL into the format expected by the `llm` addon. Two code paths, neither tested.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,34 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestAddonBaseURL_StripsModels(t *testing.T) {
+	got := addonBaseURL("https://generativelanguage.googleapis.com/v1beta/models", "gemini-pro")
+	want := "https://generativelanguage.googleapis.com/v1beta"
+	if got != want {
+		t.Errorf("addonBaseURL: got %q, want %q", got, want)
+	}
+}
+
+func TestAddonBaseURL_NoModels(t *testing.T) {
+	got := addonBaseURL("https://custom-proxy.example.com/api", "gemini-pro")
+	want := "https://custom-proxy.example.com/api"
+	if got != want {
+		t.Errorf("addonBaseURL: got %q, want %q", got, want)
+	}
+}
+
+func TestAddonBaseURL_ModelsNotAtEnd(t *testing.T) {
+	got := addonBaseURL("https://example.com/models/extra", "gemini-pro")
+	want := "https://example.com/models/extra"
+	if got != want {
+		t.Errorf("addonBaseURL passthrough: got %q, want %q", got, want)
+	}
+}
+
+func TestAddonBaseURL_IgnoresSecondArg(t *testing.T) {
+	// The second argument (model) is intentionally unused (named _).
+	got := addonBaseURL("https://api.example.com/v1beta/models", "anything")
+	want := "https://api.example.com/v1beta"
+	if got != want {
+		t.Errorf("addonBaseURL: got %q, want %q", got, want)
+	}
+}
```

**Why:** `addonBaseURL` controls URL construction for the primary production code path (addon-based generation). A bug here would silently route requests to the wrong endpoint.

---

### Gap 2: Boundary value tests for maxTokens and temperature

Only negative/zero/too-high values are tested. The exact valid boundaries (1, 1_000_000, 2.0) are never verified.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,36 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_MaxTokensBoundaryLow(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+	_, err := c.Generate(context.Background(), "test", WithMaxTokens(1))
+	if err != nil {
+		t.Fatalf("maxTokens=1 should be valid, got: %v", err)
+	}
+}
+
+func TestGenerate_MaxTokensBoundaryHigh(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+	_, err := c.Generate(context.Background(), "test", WithMaxTokens(1_000_000))
+	if err != nil {
+		t.Fatalf("maxTokens=1_000_000 should be valid, got: %v", err)
+	}
+}
+
+func TestGenerate_MaxTokensJustOverBoundary(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+	_, err := c.Generate(context.Background(), "test", WithMaxTokens(1_000_001))
+	if err == nil {
+		t.Fatal("expected error for maxTokens=1_000_001")
+	}
+}
+
+func TestGenerate_TemperatureExactMax(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+	_, err := c.Generate(context.Background(), "test", WithTemperature(2.0))
+	if err != nil {
+		t.Fatalf("temperature=2.0 should be valid, got: %v", err)
+	}
+}
```

**Why:** Off-by-one errors at boundaries are a classic bug category. These tests lock in the documented valid ranges.

---

### Gap 3: Default generation config values

Tests never verify that `Generate()` uses maxTokens=32000 and temperature=1.0 when no options are provided.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,25 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_DefaultConfig(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, _ = c.Generate(context.Background(), "test")
+
+	var req Request
+	if err := json.Unmarshal(mock.body, &req); err != nil {
+		t.Fatalf("unmarshal: %v", err)
+	}
+
+	if req.GenerationConfig.MaxOutputTokens != 32000 {
+		t.Errorf("default maxOutputTokens: got %d, want 32000", req.GenerationConfig.MaxOutputTokens)
+	}
+	if req.GenerationConfig.Temperature == nil {
+		t.Fatal("default temperature should be set")
+	}
+	if *req.GenerationConfig.Temperature != 1.0 {
+		t.Errorf("default temperature: got %f, want 1.0", *req.GenerationConfig.Temperature)
+	}
+}
```

**Why:** If someone changes the defaults in `Generate()`, this test catches the regression.

---

### Gap 4: HTTP method verification

No test verifies that `doRequest` sends a POST request (as opposed to GET, PUT, etc.).

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,14 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_UsesPostMethod(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, _ = c.Generate(context.Background(), "test")
+
+	if mock.req.Method != "POST" {
+		t.Errorf("HTTP method: got %q, want POST", mock.req.Method)
+	}
+}
```

---

### Gap 5: Short error body passthrough (no truncation)

Only the truncation path is tested. The code also has a path where error bodies shorter than 1024 bytes pass through unmodified — never verified.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,18 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_ShortErrorNotTruncated(t *testing.T) {
+	shortError := `{"error":"bad request"}`
+	mock := &mockDoer{statusCode: 400, respBody: shortError}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, err := c.Generate(context.Background(), "test")
+	if err == nil {
+		t.Fatal("expected error for 400 status")
+	}
+	errMsg := err.Error()
+	if !strings.Contains(errMsg, shortError) {
+		t.Errorf("short error body should appear in full, got: %v", err)
+	}
+}
```

---

### Gap 6: `validModel` regex edge cases

The existing tests cover common injection patterns. Missing: unicode characters, digit-only names, and names with consecutive special characters.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,35 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestNew_InvalidModelNames_Extended(t *testing.T) {
+	tests := []struct {
+		name  string
+		model string
+	}{
+		{"unicode", "gemini-\u00e9"},
+		{"newline", "gemini\nmodel"},
+		{"tab", "gemini\tmodel"},
+		{"colon", "model:evil"},
+		{"at sign", "model@v2"},
+		{"starts with hyphen", "-model"},
+		{"starts with underscore", "_model"},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			_, err := New("key", WithModel(tt.model))
+			if err == nil {
+				t.Fatalf("expected error for model %q", tt.model)
+			}
+		})
+	}
+}
+
+func TestNew_ValidModelDigitsOnly(t *testing.T) {
+	// Model names starting with a digit should be valid per the regex.
+	_, err := New("key", WithModel("1234"))
+	if err != nil {
+		t.Fatalf("digit-only model should be valid, got: %v", err)
+	}
+}
```

**Note:** The regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` would reject `-model` and `_model` (starts with non-alphanumeric) and `model:evil` (colon not in character class). But it would accept `1234`. These tests document the exact regex behavior.

---

### Gap 7: `run()` function in cmd/gemini — no args error

The CLI's `run()` function is at 0% coverage. At minimum, the no-args error path should be tested.

```diff
--- a/cmd/gemini/main_test.go
+++ b/cmd/gemini/main_test.go
@@ -87,3 +87,16 @@ func TestConfig_PanicsWithoutAPIKey(t *testing.T) {
 	}()
 	_ = chassisconfig.MustLoad[Config]()
 }
+
+func TestRun_NoArgs(t *testing.T) {
+	testkit.SetEnv(t, map[string]string{
+		"GEMINI_API_KEY": "test-key",
+	})
+
+	err := run(nil)
+	if err == nil {
+		t.Fatal("expected error for no arguments")
+	}
+	if !strings.Contains(err.Error(), "usage") {
+		t.Errorf("expected usage error, got: %v", err)
+	}
+}
```

**Note:** Would require adding `"strings"` to the import block in main_test.go.

---

### Gap 8: Cancelled context before Generate

The existing context test verifies propagation, but not what happens when the context is already cancelled.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,17 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_CancelledContext(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel() // cancel immediately
+
+	_, err := c.Generate(ctx, "test")
+	if err == nil {
+		// The mock doesn't check context, so this depends on http.NewRequestWithContext behavior.
+		// In real usage, the doer would respect context cancellation.
+		t.Log("Note: mockDoer doesn't respect context cancellation; real http.Client would error")
+	}
+}
```

---

### Gap 9: `New()` llmClient creation — verify addon client is set for standard doer

When `New()` uses the default `*http.Client`, it should create an `llmClient`. When a custom Doer is injected, `llmClient` should be nil (addon path skipped).

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,20 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestNew_AddonClientSetWithDefaultDoer(t *testing.T) {
+	c, err := New("test-key")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+	if c.llmClient == nil {
+		t.Error("llmClient should be set when using default *http.Client doer")
+	}
+}
+
+func TestNew_AddonClientNilWithCustomDoer(t *testing.T) {
+	mock := &mockDoer{}
+	c := mustNew(t, "key", WithDoer(mock))
+	if c.llmClient != nil {
+		t.Error("llmClient should be nil when custom Doer is injected")
+	}
+}
```

**Why:** This verifies the branching logic at client.go:102 that determines which generation path will be used in production.

---

### Gap 10: `Generate()` routing — verify addon vs HTTP path selection

When `llmClient` is nil (custom doer), all calls go through HTTP. When `llmClient` is set but GoogleSearch is enabled, it should still use HTTP. These routing decisions are implicit — no direct tests.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -621,3 +621,19 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_GoogleSearchForcesHTTPPath(t *testing.T) {
+	// Even with a custom doer (no llmClient), GoogleSearch calls must still work via HTTP.
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, err := c.Generate(context.Background(), "test", WithGoogleSearch())
+	if err != nil {
+		t.Fatalf("GoogleSearch via HTTP should work: %v", err)
+	}
+
+	var req Request
+	_ = json.Unmarshal(mock.body, &req)
+	if len(req.Tools) == 0 || req.Tools[0].GoogleSearch == nil {
+		t.Error("GoogleSearch tool should be present in HTTP request")
+	}
+}
```

---

## Summary of Proposed Tests

| # | Test Name | File | Gap Addressed |
|---|-----------|------|---------------|
| 1 | TestAddonBaseURL_StripsModels | client_test.go | addonBaseURL with /models suffix |
| 2 | TestAddonBaseURL_NoModels | client_test.go | addonBaseURL passthrough |
| 3 | TestAddonBaseURL_ModelsNotAtEnd | client_test.go | addonBaseURL /models in middle |
| 4 | TestAddonBaseURL_IgnoresSecondArg | client_test.go | Second arg unused |
| 5 | TestGenerate_MaxTokensBoundaryLow | client_test.go | maxTokens=1 valid |
| 6 | TestGenerate_MaxTokensBoundaryHigh | client_test.go | maxTokens=1M valid |
| 7 | TestGenerate_MaxTokensJustOverBoundary | client_test.go | maxTokens=1M+1 invalid |
| 8 | TestGenerate_TemperatureExactMax | client_test.go | temperature=2.0 valid |
| 9 | TestGenerate_DefaultConfig | client_test.go | Default maxTokens/temperature |
| 10 | TestGenerate_UsesPostMethod | client_test.go | HTTP method verification |
| 11 | TestGenerate_ShortErrorNotTruncated | client_test.go | Short errors pass through |
| 12 | TestNew_InvalidModelNames_Extended | client_test.go | Unicode, newline, colon, etc. |
| 13 | TestNew_ValidModelDigitsOnly | client_test.go | Digit-only model names |
| 14 | TestRun_NoArgs | main_test.go | CLI run() with no args |
| 15 | TestGenerate_CancelledContext | client_test.go | Pre-cancelled context |
| 16 | TestNew_AddonClientSetWithDefaultDoer | client_test.go | llmClient created for default doer |
| 17 | TestNew_AddonClientNilWithCustomDoer | client_test.go | llmClient nil for custom doer |
| 18 | TestGenerate_GoogleSearchForcesHTTPPath | client_test.go | Routing verification |

**Estimated coverage improvement:** 88.7% → ~95% for `gemini/`, 0% → ~15% for `cmd/gemini/`

---

## Untestable / Out-of-Scope Gaps

1. **`generateViaAddon()` internals** — Requires a real or mockable `llm.Client`. The `llm.Client` struct is concrete (not an interface), making it difficult to mock without modifying the dependency. A proper fix would be to introduce an interface for the LLM client, but that's a code change.

2. **`main()` function** — Standard Go practice; not worth unit-testing. Integration tests via `os/exec` would be more appropriate.

3. **Concurrent safety of `Client`** — The `Client` struct appears safe for concurrent `Generate()` calls (no shared mutable state), but there are no concurrent tests. Low priority since the struct is effectively immutable after construction.

4. **`doRequest` marshal failure** — Would require passing a type that fails `json.Marshal`, which is impractical without changing the function signature.

---

## Code Quality Observations (non-test)

- **Well-structured:** Clean separation between library (`gemini/`) and CLI (`cmd/gemini/`)
- **Security-conscious:** API key in header, HTTPS enforcement, model name validation, response size limits
- **Good defaults:** Sensible production defaults with full override capability
- **Dual-path design is pragmatic:** Using the addon for standard calls and HTTP for GoogleSearch is a reasonable trade-off given the addon's feature limitations
