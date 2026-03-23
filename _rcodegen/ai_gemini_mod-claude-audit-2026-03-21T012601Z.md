Date Created: 2026-03-21T01:26:01Z
TOTAL_SCORE: 91/100

# ai_gemini_mod — Code Audit Report

**Auditor:** Claude:Opus 4.6
**Scope:** Full codebase audit including security, code quality, testing, reliability, and maintainability
**Files Reviewed:** gemini/client.go, gemini/types.go, gemini/client_test.go, cmd/gemini/main.go, cmd/gemini/main_test.go, go.mod, .gitignore, .env

---

## Score Breakdown

| Category        | Score  | Max | Notes |
|-----------------|--------|-----|-------|
| Security        | 23     | 25  | Strong — minor API key whitespace issue |
| Code Quality    | 24     | 25  | Excellent idiomatic Go, one silent failure path |
| Testing         | 17     | 20  | Comprehensive unit tests, gaps in addon path and edge cases |
| Reliability     | 14     | 15  | Solid retry/context support, no nil context guard |
| Maintainability | 13     | 15  | Good docs/versioning, go.mod replace directives are a release hazard |
| **TOTAL**       | **91** | **100** | |

---

## Findings

### SECURITY

#### SEC-1: API Key Stored with Whitespace (Low)
**File:** `gemini/client.go:75-79`
**Severity:** Low
**Description:** The API key is validated via `strings.TrimSpace(apiKey) == ""` but then stored as-is. A key like `" sk-abc "` passes validation but the leading/trailing whitespace is sent in the `x-goog-api-key` header, which will cause silent authentication failures at the API.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -72,11 +72,12 @@ func WithTimeout(d time.Duration) Option {

 // New creates a Gemini client with the given API key and options.
 func New(apiKey string, opts ...Option) (*Client, error) {
-	if strings.TrimSpace(apiKey) == "" {
+	apiKey = strings.TrimSpace(apiKey)
+	if apiKey == "" {
 		return nil, errors.New("gemini: API key must not be empty")
 	}
 	c := &Client{
-		apiKey:  apiKey,
+		apiKey:  apiKey, // already trimmed
 		model:   defaultModel,
 		baseURL: defaultBaseURL,
 		doer:    &http.Client{Timeout: defaultTimeout},
```

#### SEC-2: HTTPS Enforcement (Pass)
**File:** `gemini/client.go:88-90`
Base URL is correctly validated to require HTTPS. No issues.

#### SEC-3: Model Name Regex Validation (Pass)
**File:** `gemini/client.go:29`
Regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` properly prevents path traversal (`../../evil`), query injection (`model?key=evil`), and other injection vectors. Well-tested in `TestNew_InvalidModelName`.

#### SEC-4: Response Size Limits (Pass)
**File:** `gemini/client.go:269-275`
10 MB limit with `io.LimitReader` + length check correctly prevents memory exhaustion. The `+1` pattern (read limit+1, check if over limit) is the correct idiom.

#### SEC-5: Error Body Truncation (Pass)
**File:** `gemini/client.go:278-283`
Error bodies truncated at 1 KB prevents log flooding from malicious/large error responses.

#### SEC-6: API Key in Header, Not URL (Pass)
**File:** `gemini/client.go:256`
API key transmitted via `x-goog-api-key` header, never in query parameters. Prevents leakage in logs, referer headers, and browser history.

#### SEC-7: Secrets Management (Pass)
`.env` file properly gitignored (`.env.*` pattern covers sync-conflict variants). Template `.env` contains placeholder only. Sync-conflict file exists on disk but is untracked.

---

### CODE QUALITY

#### CQ-1: Silent Addon Creation Failure (Medium)
**File:** `gemini/client.go:110-114`
**Severity:** Medium
**Description:** If `llm.NewClient()` fails, the error is silently swallowed and the client falls back to hand-rolled HTTP. This could mask configuration issues (e.g., invalid API key format that the addon rejects but doRequest doesn't). At minimum, a debug-level log would help diagnose issues.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -3,6 +3,7 @@ package gemini
 import (
 	"bytes"
 	"context"
+	"log/slog"
 	"encoding/json"
 	"errors"
 	"fmt"
@@ -107,8 +108,9 @@ func New(apiKey string, opts ...Option) (*Client, error) {
 		})
 		if err == nil {
 			c.llmClient = addonClient
+		} else {
+			slog.Debug("gemini: llm addon creation failed, using HTTP fallback", "error", err)
 		}
-		// If addon creation fails (shouldn't normally), fall back silently
-		// to doRequest.
 	}

 	return c, nil
```

#### CQ-2: Duplicate Default Constants (Low)
**File:** `gemini/client.go:163-164` and `cmd/gemini/main.go:33-34`
**Severity:** Low
**Description:** Default `maxTokens` (32000) and `temperature` (1.0) are hardcoded in both `Generate()` and the CLI `Config` struct. If these need to change, two locations must be updated. Consider exporting named constants.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -18,6 +18,8 @@ const (
 	defaultBaseURL    = "https://generativelanguage.googleapis.com/v1beta/models"
 	defaultModel      = "gemini-3-pro-preview"
 	defaultTimeout    = 30 * time.Second
+	DefaultMaxTokens  = 32000
+	DefaultTemperature = 1.0
 	maxResponseBytes  = 10 * 1024 * 1024 // 10 MB
 	maxErrorBodyBytes = 1024             // truncate error bodies in messages
 	maxTemperature    = 2.0
@@ -160,8 +162,8 @@ func WithGoogleSearch() GenerateOption {
 // Generate sends a prompt to the Gemini API and returns the parsed response.
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
 	cfg := &generateConfig{
-		maxTokens:   32000,
-		temperature: 1.0,
+		maxTokens:   DefaultMaxTokens,
+		temperature: DefaultTemperature,
 	}
 	for _, o := range opts {
 		o(cfg)
```

#### CQ-3: Unused Second Parameter in addonBaseURL (Informational)
**File:** `gemini/client.go:125`
**Description:** `addonBaseURL(baseURL, _ string)` has an unused second parameter (`_`). This was likely the model name. Harmless but adds noise.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -122,7 +122,7 @@ func New(apiKey string, opts ...Option) (*Client, error) {
 // Our baseURL already looks like ".../v1beta/models", so we strip the trailing
 // "/models" to let the addon reconstruct it.
-func addonBaseURL(baseURL, _ string) string {
+func addonBaseURL(baseURL string) string {
 	if strings.HasSuffix(baseURL, "/models") {
 		return strings.TrimSuffix(baseURL, "/models")
 	}
@@ -104,7 +104,7 @@ func New(apiKey string, opts ...Option) (*Client, error) {
 			Provider: llm.Gemini,
 			APIKey:   apiKey,
 			Model:    c.model,
-			BaseURL:  addonBaseURL(c.baseURL, c.model),
+			BaseURL:  addonBaseURL(c.baseURL),
 			Doer:     hc,
 		})
```

#### CQ-4: Architecture and Patterns (Pass)
- Functional options pattern: correctly implemented
- Dependency injection via `Doer` interface: clean and testable
- Hybrid addon/HTTP approach: well-motivated fallback design
- Nil-safe `Response.Text()`: excellent defensive programming
- Temperature pointer for JSON omitempty: correct Go idiom
- `GetBody` for retry replay: properly supports chassis-go middleware

---

### TESTING

#### TEST-1: No Test for Empty Prompt (Low)
**File:** `gemini/client_test.go`
**Description:** `Generate(ctx, "")` is never tested. The API will reject it, but the client happily sends it. If prompt validation is desired, a test should enforce it.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -623,3 +623,14 @@ func TestWithTimeout_IgnoredWithCustomDoer(t *testing.T) {
 		t.Error("doer should still be the custom mock")
 	}
 }
+
+func TestGenerate_EmptyPrompt(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+
+	// Document current behavior: empty prompts are sent to API.
+	// If client-side validation is desired, this test should expect an error.
+	_, err := c.Generate(context.Background(), "")
+	if err != nil {
+		t.Fatalf("unexpected error for empty prompt: %v", err)
+	}
+}
```

#### TEST-2: No Test Coverage for generateViaAddon Path (Medium)
**File:** `gemini/client_test.go`
**Description:** All tests inject a mock `Doer`, which forces the `generateViaHTTP` path. The `generateViaAddon` path is never exercised because:
1. The mock Doer prevents addon client creation (line 102: `hc, ok := c.doer.(*http.Client)` is false for mockDoer)
2. No mock for the `llm.Client` interface

This means the response mapping logic in `generateViaAddon` (lines 188-216) is untested. Consider extracting the mapping into a testable helper or providing an integration test fixture.

#### TEST-3: No Test for 3xx/1xx HTTP Status (Informational)
**File:** `gemini/client.go:277`
**Description:** `doRequest` only checks `resp.StatusCode >= 400`. Responses in the 1xx-3xx range are treated as success and passed to JSON unmarshaling. While `http.Client` follows redirects automatically, this is worth documenting in a test.

#### TEST-4: Test Quality (Pass)
- 30+ test cases with clear naming
- Edge cases thoroughly covered (nil receiver, empty parts, multiple candidates)
- Security-relevant tests (HTTP rejection, path traversal, query injection)
- Serialization tests for temperature pointer behavior
- Retry support verified via GetBody test

---

### RELIABILITY

#### REL-1: No Context Nil Guard (Low)
**File:** `gemini/client.go:161`
**Description:** `Generate()` does not guard against a nil context. Passing `nil` will panic in `http.NewRequestWithContext`. This is consistent with Go convention (functions should not accept nil contexts per `context` package docs), but a guard could produce a better error message.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -159,6 +159,9 @@ func WithGoogleSearch() GenerateOption {

 // Generate sends a prompt to the Gemini API and returns the parsed response.
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
+	if ctx == nil {
+		return nil, errors.New("gemini: context must not be nil")
+	}
 	cfg := &generateConfig{
 		maxTokens:   32000,
 		temperature: 1.0,
```

#### REL-2: Retry and Timeout Support (Pass)
- CLI correctly calculates timeout to accommodate all retry attempts
- `GetBody` enables request body replay for retry middleware
- `call.WithRetry` + `call.WithTimeout` properly delegated to chassis-go

#### REL-3: Nil-Safe Response Handling (Pass)
`Response.Text()` handles nil receiver, empty candidates, and empty parts gracefully.

---

### MAINTAINABILITY

#### MAINT-1: go.mod Replace Directives (Medium)
**File:** `go.mod:10-12`
**Severity:** Medium
**Description:** Local path `replace` directives point to `../../chassis_suite/`. These are standard for local development but are a release hazard — anyone cloning this repo without the sibling directories will get build failures. These must be removed or commented out before tagging a release.

Consider a Makefile target or CI check:
```diff
--- /dev/null
+++ b/Makefile
@@ -0,0 +1,6 @@
+.PHONY: check-release
+check-release:
+	@if grep -q '^replace ' go.mod; then \
+		echo "ERROR: go.mod contains replace directives — remove before release"; \
+		exit 1; \
+	fi
```

#### MAINT-2: No Makefile or Build Script (Low)
**Description:** No Makefile, Taskfile, or build script exists. Common commands (`go test ./...`, `go vet ./...`, `go build ./cmd/gemini`) are undocumented outside the README.

#### MAINT-3: Sync-Conflict File on Disk (Informational)
**File:** `.sync-conflict-20260215-173103-VIHEVVL.env`
**Description:** A Syncthing sync-conflict file exists on disk. It is properly gitignored and untracked. No action needed, but it should be cleaned up locally to avoid confusion.

#### MAINT-4: Documentation and Versioning (Pass)
- README is comprehensive with examples and API reference
- CHANGELOG properly maintained with version history
- AGENTS.md provides clear guidelines
- VERSION file tracked separately from code

---

## Summary of Findings by Severity

| ID | Severity | Category | Description |
|----|----------|----------|-------------|
| SEC-1 | Low | Security | API key whitespace not trimmed before storage |
| CQ-1 | Medium | Quality | Silent addon creation failure hides config issues |
| CQ-2 | Low | Quality | Duplicate default constants (maxTokens, temperature) |
| CQ-3 | Info | Quality | Unused second parameter in addonBaseURL |
| TEST-1 | Low | Testing | No test for empty prompt |
| TEST-2 | Medium | Testing | generateViaAddon path has zero test coverage |
| TEST-3 | Info | Testing | No test for non-4xx/5xx HTTP status handling |
| REL-1 | Low | Reliability | No nil context guard |
| MAINT-1 | Medium | Maintainability | go.mod replace directives are a release hazard |
| MAINT-2 | Low | Maintainability | No Makefile or build script |
| MAINT-3 | Info | Maintainability | Sync-conflict file on disk (gitignored) |

---

## Positive Highlights

1. **Excellent security posture** — HTTPS enforcement, regex model validation, response size limits, API key in headers. This is above-average for a Go API client.
2. **Comprehensive test suite** — 30+ test cases with thorough edge case coverage including nil safety, serialization quirks, and injection attempts.
3. **Clean idiomatic Go** — Functional options, interfaces for DI, proper error wrapping with `%w`, pointer types for JSON omitempty.
4. **Well-designed hybrid architecture** — Addon client for standard calls with transparent HTTP fallback for features the addon doesn't support (GoogleSearch, custom Doers).
5. **Production-ready CLI** — Structured logging, retry policies, proper context timeout calculation, graceful registry lifecycle.

---

*Report generated by Claude:Opus 4.6*
