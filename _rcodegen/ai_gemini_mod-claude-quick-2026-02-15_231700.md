Date Created: 2026-02-15 23:17:00
Date Updated: 2026-02-17
TOTAL_SCORE: 82/100

---

# ai_gemini_mod — Quick Analysis Report

**Codebase**: Go Gemini API client library + CLI wrapper
**Files analyzed**: 5 Go source files (~690 lines total), configs, .gitignore
**Agent**: Claude:Opus 4.6

---

## 1. AUDIT — Security & Code Quality

### AUDIT-1: API key transmitted via header without TLS enforcement on Doer (Medium)

The client enforces HTTPS on `baseURL`, which is good. However, the `Doer` interface accepts any `*http.Client` or custom implementation. A caller could pass a `Doer` that follows redirects to HTTP endpoints, leaking the API key header. The current HTTPS check on `baseURL` is necessary but not sufficient if redirects are in play.

**Severity**: Medium
**Impact**: API key could leak via redirect to HTTP endpoint

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -159,6 +159,11 @@ func (c *Client) doRequest(ctx context.Context, reqBody, respBody any) error {
 	req.Header.Set("Content-Type", "application/json")
 	req.Header.Set("x-goog-api-key", c.apiKey)

+	// Prevent API key from leaking on redirects to non-HTTPS hosts.
+	req.Header.Set("x-goog-api-key", c.apiKey)
+	// NOTE: Consider setting a custom CheckRedirect on the default http.Client
+	// to reject redirects to non-HTTPS URLs, preventing API key header leakage.
+
 	// Allow retry middleware to replay the body on subsequent attempts.
 	req.GetBody = func() (io.ReadCloser, error) {
```

Better approach — add a redirect policy to the default client:

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -66,7 +66,16 @@ func New(apiKey string, opts ...Option) (*Client, error) {
 	c := &Client{
 		apiKey:  apiKey,
 		model:   defaultModel,
 		baseURL: defaultBaseURL,
-		doer:    &http.Client{Timeout: defaultTimeout},
+		doer: &http.Client{
+			Timeout: defaultTimeout,
+			CheckRedirect: func(req *http.Request, via []*http.Request) error {
+				if req.URL.Scheme != "https" {
+					return fmt.Errorf("gemini: refusing redirect to non-HTTPS URL %q", req.URL)
+				}
+				if len(via) >= 10 {
+					return fmt.Errorf("gemini: too many redirects")
+				}
+				return nil
+			},
+		},
 	}
```

### AUDIT-2: Sync-conflict files in repo root (Low) — FIXED 2026-02-17

Several `.sync-conflict-*` files exist in the working directory (Syncthing artifacts). These contain potentially sensitive data (`.env` conflict, `go.sum` conflict, source code copies). They are partially covered by `.gitignore` pattern `.env.*` but the Go source conflict files are not gitignored.

**Severity**: Low
**Impact**: Accidental commit of stale/conflicting files

```diff
--- a/.gitignore
+++ b/.gitignore
@@ -1,5 +1,6 @@
 # Secrets
 .env
 .env.local
 .env.*
+*.sync-conflict-*

```

### AUDIT-3: Empty prompt not validated (Low)

`Generate()` does not validate that the prompt string is non-empty. Sending an empty prompt wastes an API call and returns a potentially confusing response.

**Severity**: Low
**Impact**: Wasted API calls, confusing behavior

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -111,6 +111,10 @@ func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOp
 		temperature: 1.0,
 	}

+	if strings.TrimSpace(prompt) == "" {
+		return nil, errors.New("gemini: prompt must not be empty")
+	}
+
 	for _, o := range opts {
 		o(cfg)
 	}
```

---

## 2. TESTS — Proposed Unit Tests

### TEST-1: Test empty prompt rejection (after AUDIT-3 fix)

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,23 @@ func TestResponse_TextMultipleCandidates(t *testing.T) {
 	if got := r.Text(); got != "first" {
 		t.Errorf("Text() should return first candidate, got %q", got)
 	}
 }
+
+func TestGenerate_EmptyPrompt(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+	_, err := c.Generate(context.Background(), "", WithMaxTokens(100))
+	if err == nil {
+		t.Fatal("expected error for empty prompt")
+	}
+	if !strings.Contains(err.Error(), "prompt must not be empty") {
+		t.Errorf("unexpected error: %v", err)
+	}
+}
+
+func TestGenerate_WhitespaceOnlyPrompt(t *testing.T) {
+	mock := &mockDoer{statusCode: 200, respBody: `{}`}
+	c := mustNew(t, "key", WithDoer(mock))
+	_, err := c.Generate(context.Background(), "   \t\n")
+	if err == nil {
+		t.Fatal("expected error for whitespace-only prompt")
+	}
+}
```

### TEST-2: Test `run()` function error paths in main

The CLI's `run()` function lacks test coverage for its core paths: missing args, successful generation, and client construction failure.

```diff
--- a/cmd/gemini/main_test.go
+++ b/cmd/gemini/main_test.go
@@ -89,3 +89,19 @@ func TestConfig_PanicsWithoutAPIKey(t *testing.T) {
 	}()
 	_ = chassisconfig.MustLoad[Config]()
 }
+
+func TestRun_NoArgs(t *testing.T) {
+	testkit.SetEnv(t, map[string]string{
+		"GEMINI_API_KEY": "test-key",
+	})
+	err := run(nil)
+	if err == nil {
+		t.Fatal("expected error for no args")
+	}
+	if !strings.Contains(err.Error(), "usage:") {
+		t.Errorf("unexpected error: %v", err)
+	}
+}
```

Note: `strings` import would need to be added to the import block.

### TEST-3: Test `run()` with empty args slice

```diff
--- a/cmd/gemini/main_test.go
+++ b/cmd/gemini/main_test.go
@@ -107,0 +108,12 @@
+func TestRun_EmptySlice(t *testing.T) {
+	testkit.SetEnv(t, map[string]string{
+		"GEMINI_API_KEY": "test-key",
+	})
+	err := run([]string{})
+	if err == nil {
+		t.Fatal("expected error for empty args")
+	}
+	if !strings.Contains(err.Error(), "usage:") {
+		t.Errorf("unexpected error: %v", err)
+	}
+}
```

### TEST-4: Test Response.Text() with single part (fast path)

The single-part fast path in `Text()` is implicitly tested by `TestGenerate_Success`, but a dedicated test is clearer.

```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -573,3 +573,14 @@
+func TestResponse_TextSinglePart(t *testing.T) {
+	r := &Response{
+		Candidates: []Candidate{{
+			Content: ResponseContent{
+				Parts: []ResponsePart{{Text: "only one"}},
+			},
+		}},
+	}
+	if got := r.Text(); got != "only one" {
+		t.Errorf("Text(): got %q, want %q", got, "only one")
+	}
+}
```

---

## 3. FIXES — Bugs, Issues, and Code Smells

### FIX-1: `retryTotalFactor` calculation is misleading (Code Smell) — FIXED 2026-02-17

In `cmd/gemini/main.go`:
```go
retryTotalFactor = retryAttempts + 1 // initial attempt + retries
```
This is 4 (3 retries + 1 initial). Then the timeout is:
```go
cfg.Timeout * time.Duration(retryTotalFactor+1)  // = cfg.Timeout * 5
```
The `+1` on line 72 is undocumented and makes `retryTotalFactor` misleading — it's not the total factor anymore. Either the constant name or the usage should be clearer.

**Severity**: Low (correctness is fine, readability isn't)

```diff
--- a/cmd/gemini/main.go
+++ b/cmd/gemini/main.go
@@ -21,8 +21,8 @@ const (
 	retryAttempts    = 3
 	retryBaseDelay   = 500 * time.Millisecond
-	retryTotalFactor = retryAttempts + 1 // initial attempt + retries
+	retryTotalAttempts = retryAttempts + 1 // initial attempt + retries
 )

@@ -72,1 +72,2 @@
-	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalFactor+1))
+	// Extra +1 provides buffer for backoff gaps between retries.
+	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalAttempts+1))
```

### FIX-2: No content-type check on API response (Low)

`doRequest` does not verify that the response `Content-Type` is `application/json` before attempting to unmarshal. If the API returns HTML (e.g., a captive portal or CDN error page), the unmarshal error message will be confusing.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -188,6 +188,13 @@ func (c *Client) doRequest(ctx context.Context, reqBody, respBody any) error {
 		return fmt.Errorf("gemini: HTTP %d: %s", resp.StatusCode, msg)
 	}

+	ct := resp.Header.Get("Content-Type")
+	if ct != "" && !strings.HasPrefix(ct, "application/json") {
+		preview := string(body)
+		if len(preview) > 200 {
+			preview = preview[:200] + "..."
+		}
+		return fmt.Errorf("gemini: unexpected content-type %q: %s", ct, preview)
+	}
+
 	if err := json.Unmarshal(body, respBody); err != nil {
```

### FIX-3: CLI outputs full JSON instead of just the text (Design Issue)

The CLI `run()` function outputs the entire JSON response including metadata, safety ratings, and usage data. For a CLI tool, users typically want just the text. This is a design choice but worth noting.

No diff — this is intentional CLI behavior but should be documented or have a `--text-only` flag in a future version.

---

## 4. REFACTOR — Opportunities to Improve

### REFACTOR-1: Consider structured error types

All errors are returned as `fmt.Errorf` strings with "gemini:" prefix. Using typed errors (e.g., `APIError{StatusCode, Body}`) would let callers programmatically handle rate limits (429), auth errors (401), etc. without string parsing.

### REFACTOR-2: Add conversation/multi-turn support

The current `Generate()` only supports single-turn prompts. The `Request.Contents` field already supports multiple content blocks but the API only populates one. Adding a `GenerateMultiTurn(ctx, []Content, ...opts)` method would be a natural extension.

### REFACTOR-3: Consider adding a `WithTimeout` option to the client — FIXED 2026-02-17

The default HTTP client timeout is 30s, but there's no `Option` to change it without providing a full custom `Doer`. The CLI works around this by passing `call.New()` as the doer, but library-only users would benefit from `WithTimeout(d time.Duration)`.

### REFACTOR-4: Move `Response.Text()` logic to be nil-safe — FIXED 2026-02-17

`Response.Text()` handles nil/empty candidates well, but calling `Text()` on a nil `*Response` would panic. Since `Generate()` returns `(*Response, error)`, a caller who ignores the error and calls `.Text()` on a nil pointer will get a nil pointer dereference. Consider making `Text()` a standalone function that accepts `*Response`, or document this behavior.

### REFACTOR-5: Test file organization

The test file `gemini/client_test.go` at ~574 lines is getting large. Consider splitting into `client_test.go` (constructor and request tests) and `response_test.go` (response parsing and Text() tests) for maintainability.

---

## Score Breakdown

| Category | Score | Max | Notes |
|----------|-------|-----|-------|
| Security | 16 | 20 | HTTPS enforced, API key validated, model regex prevents injection. Missing redirect protection. |
| Code Quality | 18 | 20 | Clean, idiomatic Go. Good separation of concerns. Minor naming issue with retryTotalFactor. |
| Test Coverage | 17 | 20 | Excellent mock-based tests for library. CLI `run()` missing integration tests. ~30 test cases total. |
| Architecture | 16 | 20 | Good use of functional options, Doer interface, and chassis framework. Single-turn only. |
| Documentation | 15 | 20 | Good godoc comments. CHANGELOG is well-maintained. Missing README, package-level docs could be expanded. |
| **Total** | **82** | **100** | |

---

## Summary

This is a well-structured, focused Go library with good security practices (HTTPS enforcement, input validation, model name regex). Test coverage is strong for the library layer but thinner for the CLI. The main areas for improvement are: (1) redirect-based API key leakage, (2) structured error types for programmatic error handling, (3) CLI test coverage for `run()`, and (4) content-type validation on responses.
