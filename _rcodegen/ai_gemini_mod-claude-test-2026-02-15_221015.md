Date Created: 2026-02-15T22:10:15Z
TOTAL_SCORE: 72/100

# ai_gemini_mod — Unit Test Coverage Report

## Executive Summary

The `ai_gemini_mod` project has **solid existing test coverage** for its core `gemini` package (34 test functions) and basic config tests for the CLI (3 tests). However, several meaningful gaps remain — particularly around the `cmd/gemini` `run()` function, edge cases in `doRequest`, HTTP status boundary conditions, and concurrency safety. The score reflects strong foundations with room for improvement in integration-style tests and boundary coverage.

---

## Scoring Breakdown

| Category                        | Max | Score | Notes |
|---------------------------------|-----|-------|-------|
| Core library coverage           | 30  | 26    | Very strong; minor gaps in `doRequest` paths |
| CLI / `run()` coverage          | 20  | 5     | Only config loading tested; `run()` untested |
| Edge case / boundary coverage   | 15  | 12    | Good validation tests; a few gaps remain |
| Error path coverage             | 15  | 12    | Most HTTP errors covered; read-error and marshal-error untested |
| Serialization / types coverage  | 10  | 9     | Excellent omitempty and pointer tests |
| Structural quality of tests     | 10  | 8     | Clean mock pattern; no subtests for some groups |
| **TOTAL**                       | **100** | **72** | |

---

## Existing Test Inventory

### `gemini/client_test.go` — 34 test functions

| Test | What it covers |
|------|---------------|
| `TestNew_Defaults` | Default client construction |
| `TestNew_EmptyAPIKey` | Empty API key rejection |
| `TestNew_HTTPBaseURL` | HTTP (non-HTTPS) rejection |
| `TestNew_WithOptions` | Custom model/doer/baseURL |
| `TestNew_WhitespaceOnlyAPIKey` | Whitespace-only key rejection |
| `TestNew_EmptyModel` | Empty model rejection |
| `TestNew_InvalidModelName` | Path traversal, dots, spaces, query strings |
| `TestNew_ValidModelNames` | Hyphens, dots, slashes, underscores |
| `TestGenerate_RequestBuilding` | URL, headers, body construction |
| `TestGenerate_NoGoogleSearch` | Tools omitted when disabled |
| `TestGenerate_RoleSetToUser` | `role:"user"` in content |
| `TestGenerate_ModelWithSpecialChars` | URL with slashes in model |
| `TestGenerate_Success` | Full success flow + Text() + UsageMetadata |
| `TestGenerate_ContextPropagated` | Context flows to request |
| `TestGenerate_GetBody` | GetBody set for retry support |
| `TestGenerate_HTTPError` | HTTP 429 error |
| `TestGenerate_DoerError` | Network-level error |
| `TestGenerate_InvalidJSONResponse` | Malformed JSON body |
| `TestGenerate_EmptyResponseBody` | Empty 200 body |
| `TestGenerate_ResponseExceedsMaxBytes` | >10 MB response |
| `TestGenerate_ErrorBodyTruncation` | Long error body truncated at 1024 |
| `TestGenerate_NegativeMaxTokens` | maxTokens < 0 |
| `TestGenerate_ZeroMaxTokens` | maxTokens = 0 |
| `TestGenerate_NegativeTemperature` | temperature < 0 |
| `TestGenerate_ZeroTemperature` | temperature = 0.0 (valid) |
| `TestGenerate_TemperatureTooHigh` | temperature > 2.0 |
| `TestGenerate_MaxTokensTooHigh` | maxTokens > 1,000,000 |
| `TestResponse_TextNoCandidates` | Empty candidates |
| `TestResponse_TextNoParts` | Empty parts |
| `TestResponse_TextMultipleParts` | Concatenation |
| `TestResponse_TextMultipleCandidates` | First candidate returned |
| `TestGenerationConfig_OmitemptyJSON` | Zero-value marshals to `{}` |
| `TestGenerationConfig_TemperatureZeroIncluded` | `*float64(0)` not omitted |
| `TestGenerate_TemperatureZeroInRequest` | Zero temp in wire format |

### `cmd/gemini/main_test.go` — 3 test functions

| Test | What it covers |
|------|---------------|
| `TestConfig_Defaults` | All default env values |
| `TestConfig_Overrides` | All env overrides |
| `TestConfig_PanicsWithoutAPIKey` | Required key panic |

---

## Coverage Gaps & Proposed Tests

### Gap 1: `run()` function is completely untested

The `run()` function in `cmd/gemini/main.go:46-94` contains significant logic:
- Argument validation (no args → error)
- Config loading + logger creation
- `call.New()` with retry/timeout options
- `gemini.New()` client construction
- Context timeout calculation
- Google Search conditional option
- JSON pretty-print output

**Priority: HIGH**

```diff
--- a/cmd/gemini/main_test.go
+++ b/cmd/gemini/main_test.go
@@ -8,6 +8,25 @@
 	"github.com/ai8future/chassis-go/v5/testkit"
 )

+func TestRun_NoArgs(t *testing.T) {
+	testkit.SetEnv(t, map[string]string{
+		"GEMINI_API_KEY": "test-key",
+	})
+
+	err := run(nil)
+	if err == nil {
+		t.Fatal("expected error for no arguments")
+	}
+	if !strings.Contains(err.Error(), "usage:") {
+		t.Errorf("expected usage message, got: %v", err)
+	}
+}
+
+func TestRun_EmptyArgs(t *testing.T) {
+	testkit.SetEnv(t, map[string]string{
+		"GEMINI_API_KEY": "test-key",
+	})
+
+	err := run([]string{})
+	if err == nil {
+		t.Fatal("expected error for empty arguments")
+	}
+}
+
 func TestMain(m *testing.M) {
```

> **Note:** Testing the full `run()` happy path requires either a live API key or injecting a mock `Doer` into the CLI path. The current architecture couples `call.New()` directly inside `run()`, making full integration testing difficult without refactoring. The no-args tests above are safe to add without any refactoring.

---

### Gap 2: HTTP status boundary conditions (400 exactly, 399, 500, 503)

The existing tests cover only HTTP 429 and 500. The boundary at `resp.StatusCode >= 400` deserves explicit verification at 399 (success) and 400 (error).

**Priority: MEDIUM**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,39 @@
 	if got := r.Text(); got != "first" {
 		t.Errorf("Text() should return first candidate, got %q", got)
 	}
 }
+
+func TestGenerate_HTTPStatusBoundary(t *testing.T) {
+	tests := []struct {
+		name       string
+		statusCode int
+		wantErr    bool
+	}{
+		{"399 is success", 399, false},
+		{"400 is error", 400, true},
+		{"401 is error", 401, true},
+		{"500 is error", 500, true},
+		{"503 is error", 503, true},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			respBody := `{}`
+			if tt.wantErr {
+				respBody = `{"error":"test"}`
+			}
+			mock := &mockDoer{statusCode: tt.statusCode, respBody: respBody}
+			c := mustNew(t, "key", WithDoer(mock))
+
+			_, err := c.Generate(context.Background(), "test")
+			if tt.wantErr && err == nil {
+				t.Fatalf("expected error for HTTP %d", tt.statusCode)
+			}
+			if !tt.wantErr && err != nil {
+				t.Fatalf("unexpected error for HTTP %d: %v", tt.statusCode, err)
+			}
+		})
+	}
+}
```

---

### Gap 3: Empty prompt string

`Generate()` accepts an empty prompt with no validation. Whether this is intentional or a bug, it should be explicitly tested to document the behavior.

**Priority: MEDIUM**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,17 @@
+
+func TestGenerate_EmptyPrompt(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, err := c.Generate(context.Background(), "")
+	// Currently no validation on empty prompt — this documents the behavior.
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	var req Request
+	if err := json.Unmarshal(mock.body, &req); err != nil {
+		t.Fatalf("unmarshal: %v", err)
+	}
+	if req.Contents[0].Parts[0].Text != "" {
+		t.Errorf("expected empty prompt text, got %q", req.Contents[0].Parts[0].Text)
+	}
+}
```

---

### Gap 4: Cancelled context propagation

The test `TestGenerate_ContextPropagated` checks context identity but doesn't test what happens when the context is already cancelled before the call.

**Priority: MEDIUM**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,19 @@
+
+func TestGenerate_CancelledContext(t *testing.T) {
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel() // Cancel immediately.
+
+	mock := &mockDoer{
+		statusCode: 200,
+		respBody:   `{}`,
+		err:        ctx.Err(), // Simulate what http.Client does with cancelled ctx.
+	}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, err := c.Generate(ctx, "test")
+	if err == nil {
+		t.Fatal("expected error for cancelled context")
+	}
+}
```

---

### Gap 5: Boundary-exact maxTokens and temperature values

Tests exist for invalid values (negative, too high) and zero, but the exact boundary values (maxTokens=1, maxTokens=1000000, temperature=2.0) are untested.

**Priority: MEDIUM**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,33 @@
+
+func TestGenerate_BoundaryMaxTokens(t *testing.T) {
+	tests := []struct {
+		name    string
+		tokens  int
+		wantErr bool
+	}{
+		{"min valid (1)", 1, false},
+		{"max valid (1000000)", 1_000_000, false},
+		{"one over max (1000001)", 1_000_001, true},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			mock := &mockDoer{statusCode: 200, respBody: `{}`}
+			c := mustNew(t, "key", WithDoer(mock))
+			_, err := c.Generate(context.Background(), "test", WithMaxTokens(tt.tokens))
+			if tt.wantErr && err == nil {
+				t.Fatalf("expected error for maxTokens=%d", tt.tokens)
+			}
+			if !tt.wantErr && err != nil {
+				t.Fatalf("unexpected error for maxTokens=%d: %v", tt.tokens, err)
+			}
+		})
+	}
+}
+
+func TestGenerate_BoundaryTemperature(t *testing.T) {
+	tests := []struct {
+		name    string
+		temp    float64
+		wantErr bool
+	}{
+		{"exact max (2.0)", 2.0, false},
+		{"just over max (2.01)", 2.01, true},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			mock := &mockDoer{statusCode: 200, respBody: `{}`}
+			c := mustNew(t, "key", WithDoer(mock))
+			_, err := c.Generate(context.Background(), "test", WithTemperature(tt.temp))
+			if tt.wantErr && err == nil {
+				t.Fatalf("expected error for temperature=%f", tt.temp)
+			}
+			if !tt.wantErr && err != nil {
+				t.Fatalf("unexpected error for temperature=%f: %v", tt.temp, err)
+			}
+		})
+	}
+}
```

---

### Gap 6: Default Generate config values (no options passed)

When `Generate()` is called with no options, the defaults (maxTokens=32000, temperature=1.0) are used. This is tested implicitly by `TestGenerate_Success` but the actual default wire values are never asserted.

**Priority: LOW**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,24 @@
+
+func TestGenerate_DefaultConfig(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	_, _ = c.Generate(context.Background(), "test") // No generate options.
+
+	var req Request
+	if err := json.Unmarshal(mock.body, &req); err != nil {
+		t.Fatalf("unmarshal: %v", err)
+	}
+	if req.GenerationConfig.MaxOutputTokens != 32000 {
+		t.Errorf("default maxOutputTokens: got %d, want 32000",
+			req.GenerationConfig.MaxOutputTokens)
+	}
+	if req.GenerationConfig.Temperature == nil {
+		t.Fatal("expected temperature to be set")
+	}
+	if *req.GenerationConfig.Temperature != 1.0 {
+		t.Errorf("default temperature: got %f, want 1.0",
+			*req.GenerationConfig.Temperature)
+	}
+}
```

---

### Gap 7: Error body at exactly `maxErrorBodyBytes` (1024)

The truncation test uses a 2000-char body (well over 1024). A boundary test at exactly 1024 and 1025 would be more precise.

**Priority: LOW**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,33 @@
+
+func TestGenerate_ErrorBodyTruncationBoundary(t *testing.T) {
+	tests := []struct {
+		name        string
+		bodyLen     int
+		wantTrunc   bool
+	}{
+		{"exact limit (1024)", 1024, false},
+		{"one over (1025)", 1025, true},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			body := strings.Repeat("E", tt.bodyLen)
+			mock := &mockDoer{statusCode: 500, respBody: body}
+			c := mustNew(t, "key", WithDoer(mock))
+
+			_, err := c.Generate(context.Background(), "test")
+			if err == nil {
+				t.Fatal("expected error")
+			}
+			errMsg := err.Error()
+			hasTrunc := strings.Contains(errMsg, "...(truncated)")
+			if tt.wantTrunc && !hasTrunc {
+				t.Error("expected truncation marker")
+			}
+			if !tt.wantTrunc && hasTrunc {
+				t.Error("did not expect truncation marker")
+			}
+		})
+	}
+}
```

---

### Gap 8: `Response.Text()` with single part (fast path)

`Text()` has an optimization at line 89-91 of `types.go` for single-part responses that avoids `strings.Builder`. The existing `TestGenerate_Success` hits this indirectly, but a dedicated unit test documents and protects this optimization.

**Priority: LOW**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,13 @@
+
+func TestResponse_TextSinglePart(t *testing.T) {
+	r := &Response{
+		Candidates: []Candidate{{
+			Content: ResponseContent{
+				Parts: []ResponsePart{{Text: "only part"}},
+			},
+		}},
+	}
+	if got := r.Text(); got != "only part" {
+		t.Errorf("Text(): got %q, want %q", got, "only part")
+	}
+}
```

---

### Gap 9: Request JSON serialization round-trip for `Tool` and `GoogleSearch`

The `tools` field uses `omitempty`. When `GoogleSearch` is enabled, the serialized form should include `{"googleSearch":{}}`. This is partially tested in `TestGenerate_RequestBuilding` but deserves an isolated serialization test.

**Priority: LOW**

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,25 @@
+
+func TestToolSerialization(t *testing.T) {
+	t.Run("with google search", func(t *testing.T) {
+		req := Request{
+			Contents: []Content{{Role: "user", Parts: []Part{{Text: "test"}}}},
+			Tools:    []Tool{{GoogleSearch: &GoogleSearch{}}},
+		}
+		data, err := json.Marshal(req)
+		if err != nil {
+			t.Fatalf("marshal: %v", err)
+		}
+		if !strings.Contains(string(data), `"googleSearch"`) {
+			t.Errorf("expected googleSearch in JSON, got %s", data)
+		}
+	})
+
+	t.Run("without tools", func(t *testing.T) {
+		req := Request{
+			Contents: []Content{{Role: "user", Parts: []Part{{Text: "test"}}}},
+		}
+		data, err := json.Marshal(req)
+		if err != nil {
+			t.Fatalf("marshal: %v", err)
+		}
+		if strings.Contains(string(data), `"tools"`) {
+			t.Errorf("expected no tools key in JSON, got %s", data)
+		}
+	})
+}
```

---

### Gap 10: Multiple `GenerateOption` combination test

No test exercises combining all three generate options simultaneously and then verifying the request body reflects all of them. `TestGenerate_RequestBuilding` does this, but a dedicated options-combo test would be cleaner.

**Priority: LOW** — Already covered by `TestGenerate_RequestBuilding`.

---

## Structural Observations

### Strengths
1. **Clean `Doer` interface** — enables thorough mocking without test servers
2. **`mustNew` helper** — reduces boilerplate
3. **Subtests for table-driven tests** — `TestNew_InvalidModelName`, `TestNew_ValidModelNames`
4. **Security-first validation** — HTTPS enforcement, model name regex, API key checks
5. **Serialization edge cases** — pointer-based `*float64` for temperature zero-value handling

### Weaknesses
1. **`run()` is untestable without refactoring** — `call.New()` and `gemini.New()` are hard-coded inside, no way to inject a mock Doer for the full CLI path
2. **No table-driven approach for HTTP status codes** — each status code gets its own test function
3. **No concurrency tests** — `Client` is presumably safe for concurrent use (stateless `Generate`), but this is never verified
4. **Sync conflict files** — 4 `.sync-conflict-*` files in the repo prevent `go build` and `go test` from succeeding

### Build Blocker

The following sync conflict files cause duplicate type declarations and must be removed before any tests can run:

- `gemini/types.sync-conflict-20260215-173222-VIHEVVL.go`
- `gemini/client_test.sync-conflict-20260215-173043-VIHEVVL.go`
- `go.sync-conflict-20260215-173017-VIHEVVL.sum`
- `.sync-conflict-20260215-173103-VIHEVVL.env`

---

## Summary of Proposed New Tests

| # | Test Name | File | Priority | Lines of Code |
|---|-----------|------|----------|---------------|
| 1 | `TestRun_NoArgs` | `cmd/gemini/main_test.go` | HIGH | ~12 |
| 2 | `TestRun_EmptyArgs` | `cmd/gemini/main_test.go` | HIGH | ~8 |
| 3 | `TestGenerate_HTTPStatusBoundary` | `gemini/client_test.go` | MEDIUM | ~30 |
| 4 | `TestGenerate_EmptyPrompt` | `gemini/client_test.go` | MEDIUM | ~16 |
| 5 | `TestGenerate_CancelledContext` | `gemini/client_test.go` | MEDIUM | ~15 |
| 6 | `TestGenerate_BoundaryMaxTokens` | `gemini/client_test.go` | MEDIUM | ~22 |
| 7 | `TestGenerate_BoundaryTemperature` | `gemini/client_test.go` | MEDIUM | ~22 |
| 8 | `TestGenerate_DefaultConfig` | `gemini/client_test.go` | LOW | ~18 |
| 9 | `TestGenerate_ErrorBodyTruncationBoundary` | `gemini/client_test.go` | LOW | ~26 |
| 10 | `TestResponse_TextSinglePart` | `gemini/client_test.go` | LOW | ~10 |
| 11 | `TestToolSerialization` | `gemini/client_test.go` | LOW | ~22 |

**Total: 11 new tests (~200 lines of test code)**

---

## Final Assessment

The codebase has a disciplined testing culture — the `gemini` package in particular has thorough coverage of its public API surface. The main deduction comes from the completely untested `run()` function (which contains real business logic around retry config, context timeouts, and conditional options), the lack of boundary-exact tests, and the build-blocking sync conflict files. Adding the proposed tests would bring the score to approximately **85-88/100**. Reaching 90+ would require refactoring `run()` to accept injected dependencies for full integration testing.
