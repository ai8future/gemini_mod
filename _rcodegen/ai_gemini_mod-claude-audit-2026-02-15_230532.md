Date Created: 2026-02-15 23:05:32 UTC
TOTAL_SCORE: 78/100

# Audit Report: ai_gemini_mod

**Auditor**: Claude Opus 4.6
**Codebase**: ai_gemini_mod (Gemini API client library + CLI)
**Version Audited**: 1.2.0 (uncommitted working tree)
**Go Version**: 1.25.5
**Framework**: chassis-go v5.0.0

---

## Executive Summary

This is a well-structured Go client library for the Google Gemini API with a CLI wrapper. The code demonstrates solid patterns: functional options, dependency injection via the `Doer` interface, input validation, and good test coverage. However, the audit uncovered several issues ranging from **critical** (broken build due to sync-conflict files, API key exposure in logs) to **moderate** (missing prompt validation, hardcoded defaults duplicated across packages) and **minor** (style/hygiene issues).

---

## Scoring Breakdown

| Category                  | Weight | Score | Notes                                                    |
|---------------------------|--------|-------|----------------------------------------------------------|
| Security                  | 25     | 18/25 | API key in header (fine), but key loggable; no prompt sanitization |
| Correctness / Bug Risk    | 25     | 19/25 | Build broken by sync-conflict files; timeout math is fragile |
| Code Quality / Structure  | 20     | 18/20 | Clean patterns; minor duplication of defaults             |
| Test Coverage             | 15     | 13/15 | 30+ tests, good edge cases; missing empty-prompt test     |
| Documentation / Hygiene   | 10     | 6/10  | No README; sync-conflict files polluting repo             |
| Dependency Management     | 5      | 4/5   | Clean go.mod; indirect deps are reasonable                |
| **TOTAL**                 | **100**| **78**|                                                          |

---

## Findings

### CRITICAL

#### C1. Sync-Conflict Files Break the Build
**Files**: `gemini/types.sync-conflict-*.go`, `gemini/client_test.sync-conflict-*.go`, `go.sync-conflict-*.sum`, `.sync-conflict-*.env`
**Impact**: `go test ./...` and `go build ./...` fail with redeclaration errors. The Go toolchain picks up all `.go` files in a package directory. These Syncthing conflict files are valid Go source that redeclares every type.
**Severity**: Critical - CI/CD and any developer checkout is broken.

```diff
--- /dev/null
+++ .gitignore
@@ -1,4 +1,5 @@
 # Secrets
 .env
 .env.local
 .env.*
+*.sync-conflict-*
```

And delete the four offending files:
```bash
rm gemini/client_test.sync-conflict-20260215-173043-VIHEVVL.go
rm gemini/types.sync-conflict-20260215-173222-VIHEVVL.go
rm go.sync-conflict-20260215-173017-VIHEVVL.sum
rm .sync-conflict-20260215-173103-VIHEVVL.env
```

---

#### C2. API Key Potentially Logged via Debug Output
**File**: `cmd/gemini/main.go:56`
**Issue**: The debug log line logs `cfg.Model`, `cfg.MaxTokens`, `cfg.Temperature` but not `cfg.APIKey`. That's good. However, the `cfg` struct itself could be logged elsewhere (e.g., by chassis middleware, or if a future developer adds `logger.Debug("config", "cfg", cfg)`). The API key is stored as a plain `string` field with no redaction mechanism.
**Severity**: Critical (potential) - API key leakage in logs is a common production incident.

```diff
--- a/cmd/gemini/main.go
+++ b/cmd/gemini/main.go
@@ -27,7 +27,7 @@
 type Config struct {
-	APIKey       string        `env:"GEMINI_API_KEY" required:"true"`
+	APIKey       string        `env:"GEMINI_API_KEY" required:"true" json:"-" logz:"-"`
 	Model        string        `env:"GEMINI_MODEL" default:"gemini-3-pro-preview"`
```

Additionally, consider implementing `fmt.Stringer` or `fmt.GoStringer` on `Config` to redact the key if the struct is ever printed:

```diff
--- a/cmd/gemini/main.go
+++ b/cmd/gemini/main.go
@@ -35,6 +35,13 @@
 	LogLevel     string        `env:"LOG_LEVEL" default:"error"`
 }

+// String redacts the API key when Config is printed.
+func (c Config) String() string {
+	safe := c
+	safe.APIKey = "[REDACTED]"
+	return fmt.Sprintf("%+v", safe)
+}
+
 func main() {
```

---

### HIGH

#### H1. Empty Prompt Accepted Without Validation
**File**: `gemini/client.go:111`
**Issue**: `Generate()` does not validate that `prompt` is non-empty. An empty string will be sent to the Gemini API, consuming a round-trip and possibly quota, only to get back an empty or error response. This wastes resources and produces confusing error messages from the upstream API.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -110,6 +110,10 @@
 // Generate sends a prompt to the Gemini API and returns the parsed response.
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
+	if strings.TrimSpace(prompt) == "" {
+		return nil, errors.New("gemini: prompt must not be empty")
+	}
+
 	cfg := &generateConfig{
```

---

#### H2. Default Values Duplicated Between Library and CLI
**File**: `gemini/client.go:112-114` vs `cmd/gemini/main.go:29-31`
**Issue**: The default `maxTokens` (32000) and `temperature` (1.0) are hardcoded in both `Generate()` and in the CLI `Config` struct's `default` tags. If either changes without updating the other, behavior diverges silently. The library defaults silently override CLI defaults when `WithMaxTokens`/`WithTemperature` are not passed.

Since the CLI always passes these options explicitly, the library defaults at lines 113-114 are actually dead code in the CLI path. But library-only consumers get different defaults if not documented.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -110,10 +110,9 @@
 // Generate sends a prompt to the Gemini API and returns the parsed response.
+// Callers should provide WithMaxTokens and WithTemperature explicitly.
+// Library defaults: maxTokens=32000, temperature=1.0.
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
 	cfg := &generateConfig{
-		maxTokens:   32000,
-		temperature: 1.0,
+		maxTokens:   defaultMaxTokens,
+		temperature: defaultTemperature,
 	}
```

And define named constants:
```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -16,6 +16,8 @@
 const (
 	defaultBaseURL    = "https://generativelanguage.googleapis.com/v1beta/models"
 	defaultModel      = "gemini-3-pro-preview"
+	defaultMaxTokens  = 32000
+	defaultTemperature = 1.0
 	defaultTimeout    = 30 * time.Second
```

---

#### H3. Context Timeout Math Is Fragile
**File**: `cmd/gemini/main.go:23-24,72`
**Issue**: The timeout multiplier `retryTotalFactor = retryAttempts + 1` (= 4), then used as `cfg.Timeout * (retryTotalFactor + 1)` (= 5x), is an approximation that doesn't account for exponential backoff. With 3 retries and 500ms base delay, the total backoff is ~500ms + ~1000ms + ~2000ms = ~3.5s. The context timeout is 5 * 30s = 150s, which is generous but the math is misleading and the comment says "Allow enough time for all retry attempts plus backoff gaps" without proving it.

More importantly, `call.New()` at line 58 sets its own timeout via `call.WithTimeout(cfg.Timeout)` per attempt, plus its own retry backoff. If `call.Client` also wraps with its own context deadline, these two layers of timeouts may interact unpredictably.

```diff
--- a/cmd/gemini/main.go
+++ b/cmd/gemini/main.go
@@ -69,8 +69,10 @@
 	}

-	// Allow enough time for all retry attempts plus backoff gaps.
-	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalFactor+1))
+	// Context deadline must exceed: (retryAttempts+1) * per-attempt timeout + total backoff.
+	// Backoff: 500ms + 1s + 2s = 3.5s. Per-attempt: 30s * 4 = 120s. Total ~124s. Use 2x safety.
+	totalBudget := cfg.Timeout*time.Duration(retryAttempts+1) + retryBaseDelay*7 // 7 = 1+2+4
+	ctx, cancel := context.WithTimeout(context.Background(), totalBudget)
 	defer cancel()
```

---

### MODERATE

#### M1. No `go vet` / `staticcheck` in CI Configuration
**Issue**: No CI/CD configuration file (Makefile, GitHub Actions, etc.) was found. Static analysis is not enforced. The sync-conflict file breakage (C1) would have been caught immediately by any CI pipeline.

Recommend adding a Makefile:
```diff
--- /dev/null
+++ b/Makefile
@@ -0,0 +1,12 @@
+.PHONY: test vet lint build
+
+test:
+	go test ./...
+
+vet:
+	go vet ./...
+
+build:
+	go build ./cmd/gemini
+
+all: vet test build
```

---

#### M2. HTTP Status Codes 300-399 (Redirects) Not Handled
**File**: `gemini/client.go:183`
**Issue**: Only status codes >= 400 are treated as errors. Go's `http.Client` follows redirects by default (up to 10), but if the API were to return a 3xx that isn't followed (e.g., 304 Not Modified, or redirect limit reached), the response body would be parsed as a valid `Response`, likely producing a silent empty result.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -182,6 +182,9 @@
 	if len(body) > maxResponseBytes {
 		return fmt.Errorf("gemini: response exceeds %d byte limit", maxResponseBytes)
 	}
+	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
+		// Treat any non-2xx as an error (3xx not expected from Gemini API).
+	}

 	if resp.StatusCode >= 400 {
```

Better consolidated version:
```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -183,7 +183,7 @@
 		return fmt.Errorf("gemini: response exceeds %d byte limit", maxResponseBytes)
 	}

-	if resp.StatusCode >= 400 {
+	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
 		msg := string(body)
 		if len(msg) > maxErrorBodyBytes {
 			msg = msg[:maxErrorBodyBytes] + "...(truncated)"
```

---

#### M3. No Prompt Length Validation / Size Limit
**File**: `gemini/client.go:111`
**Issue**: There's no upper bound on prompt length. A caller could pass a multi-GB string, which would be marshaled into JSON and sent over the wire. The response has a 10MB limit, but the request has none.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -16,6 +16,7 @@
 const (
+	maxPromptBytes    = 1024 * 1024 // 1 MB - Gemini's actual limit is model-dependent
 	defaultBaseURL    = "https://generativelanguage.googleapis.com/v1beta/models"
@@ -110,6 +111,9 @@
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
+	if len(prompt) > maxPromptBytes {
+		return nil, fmt.Errorf("gemini: prompt exceeds %d byte limit", maxPromptBytes)
+	}
```

---

#### M4. Model Name Regex Allows Trailing Slashes and Double-Dots
**File**: `gemini/client.go:27`
**Regex**: `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`
**Issue**: This regex allows `models/../../etc/passwd` as long as it starts with an alphanumeric char. The `..` sequence is explicitly tested and rejected in the test suite via `../../evil`, but the regex itself allows models like `a../b` or `a/./b`. The test only catches `../../evil` because it starts with `.`.

However, a model like `a/../../evil` would pass validation. Let's verify:
- `a/../../evil` -> starts with `a` (alphanumeric), remaining `/../../evil` matches `[a-zA-Z0-9._/-]*` -> **PASSES validation**.
- The URL becomes `.../models/a/../../evil:generateContent` which resolves to `.../evil:generateContent` - a path traversal!

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -26,7 +26,7 @@

-// validModel matches model names: alphanumeric, dots, hyphens, underscores, slashes.
-var validModel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)
+// validModel matches model names: alphanumeric segments separated by single slashes.
+// Rejects "..", "//", leading/trailing slashes, and other path traversal patterns.
+var validModel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*(/[a-zA-Z0-9][a-zA-Z0-9._-]*)*$`)
```

This ensures:
- Each path segment starts with an alphanumeric character
- No `..` traversal (`.` is allowed mid-segment but consecutive dots followed by `/` can't start a segment)
- No `//` double slashes
- No trailing `/`

Add test cases:
```diff
--- a/gemini/client_test.go
+++ b/gemini/client_test.go
@@ -366,6 +366,8 @@
 		{"starts with dot", ".hidden"},
 		{"spaces", "model name"},
 		{"query injection", "model?key=evil"},
+		{"path traversal mid", "a/../../evil"},
+		{"double slash", "models//gemini"},
+		{"trailing slash", "models/"},
 	}
```

---

#### M5. Error Body Truncation Uses Byte Count on String
**File**: `gemini/client.go:184-187`
**Issue**: `msg[:maxErrorBodyBytes]` slices a string at byte offset 1024. If the error body is UTF-8 (likely, as it's JSON from Google), this could slice in the middle of a multi-byte character, producing invalid UTF-8 in the error message.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -184,7 +184,11 @@
 		msg := string(body)
 		if len(msg) > maxErrorBodyBytes {
-			msg = msg[:maxErrorBodyBytes] + "...(truncated)"
+			// Truncate at a valid UTF-8 boundary.
+			truncated := msg[:maxErrorBodyBytes]
+			for len(truncated) > 0 && truncated[len(truncated)-1]&0xC0 == 0x80 {
+				truncated = truncated[:len(truncated)-1]
+			}
+			msg = truncated + "...(truncated)"
 		}
```

Alternatively, use the `unicode/utf8` package:
```diff
+	import "unicode/utf8"
+	...
+	if len(msg) > maxErrorBodyBytes {
+		for maxErrorBodyBytes > 0 && !utf8.RuneStart(msg[maxErrorBodyBytes]) {
+			maxErrorBodyBytes--
+		}
+		msg = msg[:maxErrorBodyBytes] + "...(truncated)"
+	}
```

---

### LOW

#### L1. CLI Outputs Full JSON Response (Information Disclosure)
**File**: `cmd/gemini/main.go:88-92`
**Issue**: The CLI prints the entire JSON response including `safetyRatings`, `usageMetadata`, and `finishReason`. For a user-facing tool, this leaks internal API metadata. Consider outputting just `resp.Text()` by default with a `--json` flag for the full response.

No patch provided - this is a design decision.

---

#### L2. No `User-Agent` Header
**File**: `gemini/client.go:161-162`
**Issue**: Requests to the Gemini API don't include a `User-Agent` header. This makes debugging harder on Google's side and is considered poor API citizenship.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -161,6 +161,7 @@
 	req.Header.Set("Content-Type", "application/json")
 	req.Header.Set("x-goog-api-key", c.apiKey)
+	req.Header.Set("User-Agent", "ai_gemini_mod/1.2.0 Go")
```

---

#### L3. `GenerateConfig` `maxTokens` Default Conflicts With Zero-Value
**File**: `gemini/client.go:89-93`
**Issue**: The `generateConfig` struct uses `int` for `maxTokens` with default 32000 set in `Generate()`. If a caller passes `WithMaxTokens(0)`, it's rejected. But the zero-value of `int` is 0, meaning you can't distinguish "caller didn't set" from "caller set to 0". Currently this is handled correctly (validation rejects 0), but a pointer or sentinel value would be more explicit.

No patch - the current behavior is correct, just worth noting for future maintainers.

---

#### L4. `.env` File Contains Placeholder Key
**File**: `.env:1`
**Issue**: The `.env` file is gitignored (good), but it's currently tracked with `GEMINI_API_KEY=your-api-key-here`. This is fine as a template, but the CHANGELOG mentions a leaked API key was previously in this file (v1.2.0). Ensure the committed version never had a real key by checking git history.

```bash
git log --all -p -- .env | grep -v "your-api-key-here" | grep "GEMINI_API_KEY="
```

---

#### L5. Test Helper `mustNew` Not Reused in All Tests
**File**: `gemini/client_test.go:99-106`
**Issue**: `mustNew()` is defined but not used in all tests that create clients (e.g., `TestNew_Defaults` at line 36 duplicates the pattern). Minor inconsistency.

No patch - cosmetic only.

---

#### L6. `TestMain` in `cmd/gemini/main_test.go` Adds Unnecessary Chassis Gate
**File**: `cmd/gemini/main_test.go:12-15`
**Issue**: `chassis.RequireMajor(5)` in `TestMain` will panic and abort all tests if the chassis version doesn't match. This is already enforced in the production `main()`. Having it in tests means test failures are reported as panics rather than clear assertion failures.

No patch - minor concern.

---

## Test Results

Tests pass when sync-conflict files are removed:
```
ok  	ai_gemini_mod/cmd/gemini	0.354s
ok  	ai_gemini_mod/gemini	0.637s
```

Tests **FAIL** in the actual working tree due to C1 (sync-conflict files).

---

## Missing Tests

| Area | Description |
|------|-------------|
| Empty prompt | No test for `Generate(ctx, "")` or `Generate(ctx, "   ")` |
| Path traversal mid-string | `a/../../evil` model name passes current regex |
| Double-slash model | `models//gemini` model name not tested |
| Trailing slash model | `models/` model name not tested |
| Concurrent Generate calls | No concurrency/race test |
| Context cancellation during request | No test for cancelled context behavior |
| Very large prompt | No test for prompt size limits |
| Non-2xx success status (e.g., 201) | Not tested (would succeed, but unexpected from Gemini) |

---

## Positive Observations

1. **Functional options pattern** is idiomatic and well-implemented
2. **Doer interface** enables excellent testability via mock injection
3. **HTTPS enforcement** prevents accidental plaintext API key transmission
4. **Response size limit** (10MB) prevents OOM from malicious/buggy responses
5. **Error body truncation** prevents log flooding
6. **`GetBody` support** enables retry middleware to replay request bodies
7. **Temperature as `*float64`** correctly distinguishes "not set" from "set to 0.0"
8. **30+ test cases** with edge cases, validation, and serialization coverage
9. **Clean dependency tree** - only chassis-go as direct dependency
10. **Consistent error message prefix** (`gemini:`) aids log parsing

---

## Recommendations Priority

| Priority | Finding | Effort |
|----------|---------|--------|
| 1        | C1: Delete sync-conflict files, update .gitignore | 5 min |
| 2        | M4: Fix model name regex (path traversal) | 15 min |
| 3        | H1: Add empty prompt validation | 5 min |
| 4        | C2: Add API key redaction | 15 min |
| 5        | M2: Handle non-2xx status codes | 5 min |
| 6        | H2: Extract magic numbers to named constants | 10 min |
| 7        | H3: Fix timeout math | 10 min |
| 8        | M5: Fix UTF-8 truncation | 10 min |
| 9        | M3: Add prompt size limit | 5 min |
| 10       | M1: Add Makefile / CI | 30 min |
