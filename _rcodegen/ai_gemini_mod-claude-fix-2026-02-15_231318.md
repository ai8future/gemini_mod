Date Created: 2026-02-15 23:13:18
TOTAL_SCORE: 82/100

# ai_gemini_mod Code Audit Report

**Auditor:** Claude:Opus 4.6
**Version Audited:** 1.2.0
**Files Reviewed:** gemini/client.go, gemini/types.go, gemini/client_test.go, cmd/gemini/main.go, cmd/gemini/main_test.go, go.mod, CHANGELOG.md, .gitignore

---

## Executive Summary

This is a well-structured, cleanly written Go module wrapping the Google Gemini API. The code demonstrates good security awareness (HTTPS enforcement, model name validation, response size limiting, API key in header not URL) and solid testing. The functional options pattern is idiomatic Go. There are several issues worth addressing, ranging from a real bug to minor code smells.

---

## Issues Found

### BUG-1: API key stored untrimmed despite trimmed validation (Severity: Low)

**File:** `gemini/client.go:62-64`

The constructor validates that the API key is non-empty after trimming, but stores the original (potentially whitespace-padded) value. If a user passes `" sk-abc123 "`, validation passes but the API key sent in the `x-goog-api-key` header includes leading/trailing whitespace, which would cause auth failures at the API level.

**Current code:**
```go
func New(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("gemini: API key must not be empty")
	}
	c := &Client{
		apiKey:  apiKey,  // stores untrimmed value
```

**Patch:**
```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -59,10 +59,11 @@

 // New creates a Gemini client with the given API key and options.
 func New(apiKey string, opts ...Option) (*Client, error) {
-	if strings.TrimSpace(apiKey) == "" {
+	apiKey = strings.TrimSpace(apiKey)
+	if apiKey == "" {
 		return nil, errors.New("gemini: API key must not be empty")
 	}
 	c := &Client{
-		apiKey:  apiKey,
+		apiKey:  apiKey,  // now trimmed
 		model:   defaultModel,
 		baseURL: defaultBaseURL,
```

---

### BUG-2: Empty prompt is silently accepted (Severity: Low)

**File:** `gemini/client.go:111`

`Generate()` accepts an empty prompt string without error. This will send a valid-looking request to the Gemini API with `"text": ""`, which is almost certainly a caller mistake. The CLI guards against this with `len(args) == 0`, but the library itself doesn't validate.

**Patch:**
```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -109,6 +109,10 @@

 // Generate sends a prompt to the Gemini API and returns the parsed response.
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
+	if strings.TrimSpace(prompt) == "" {
+		return nil, errors.New("gemini: prompt must not be empty")
+	}
+
 	cfg := &generateConfig{
 		maxTokens:   32000,
 		temperature: 1.0,
```

---

### SMELL-1: Magic number duplication for default maxTokens (Severity: Low)

**File:** `gemini/client.go:113` and `cmd/gemini/main.go:30`

The default maxTokens value `32000` appears as a magic number in `Generate()` and again as a config default in `main.go`. If these drift apart, the CLI and library would silently use different defaults.

**Recommendation:** Export a constant `DefaultMaxTokens = 32000` from the `gemini` package and reference it in both places. Same for `DefaultTemperature = 1.0`.

**Patch:**
```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -16,6 +16,10 @@
 const (
 	defaultBaseURL    = "https://generativelanguage.googleapis.com/v1beta/models"
 	defaultModel      = "gemini-3-pro-preview"
+	// DefaultMaxTokens is the default maximum output tokens for a Generate call.
+	DefaultMaxTokens  = 32000
+	// DefaultTemperature is the default temperature for a Generate call.
+	DefaultTemperature = 1.0
 	defaultTimeout    = 30 * time.Second
 	maxResponseBytes  = 10 * 1024 * 1024 // 10 MB
 	maxErrorBodyBytes = 1024             // truncate error bodies in messages
@@ -110,8 +114,8 @@
 func (c *Client) Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error) {
 	cfg := &generateConfig{
-		maxTokens:   32000,
-		temperature: 1.0,
+		maxTokens:   DefaultMaxTokens,
+		temperature: DefaultTemperature,
 	}
```

---

### SMELL-2: `WithModel` and `WithBaseURL` options bypass validation (Severity: Medium)

**File:** `gemini/client.go:46-58`

The `WithModel` and `WithBaseURL` options set values without validation. Validation happens after all options are applied in `New()`, which is correct. However, these same Option types could hypothetically be stored and re-applied after construction (the type is exported). More importantly, `WithBaseURL` doesn't trim trailing slashes, which would produce URLs like `https://api.test//model:generateContent` with a double slash. This is cosmetically wrong but usually tolerated by HTTP servers.

**Patch (trim trailing slash from base URL):**
```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -55,7 +55,7 @@

 // WithBaseURL overrides the API base URL.
 func WithBaseURL(url string) Option {
-	return func(c *Client) { c.baseURL = url }
+	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
 }
```

---

### SMELL-3: `VERSION.chassis` says `5.0.0` but actual go.mod says `chassis-go/v5 v5.0.0` (Severity: Info)

**File:** `VERSION.chassis` contains `5.0.0`, but the CHANGELOG says `1.4.0` was the chassis module version when it was introduced. The VERSION.chassis file appears to now track the chassis major version rather than the chassis module release. This is just a documentation/convention inconsistency - not a real bug.

---

### SMELL-4: No `context.Context` nil check in `Generate` (Severity: Very Low)

**File:** `gemini/client.go:111`

If a caller passes `nil` for `ctx`, `http.NewRequestWithContext` will panic. This is consistent with Go standard library conventions (callers must not pass nil context), but a defensive check could prevent confusing panics.

**Note:** This is extremely minor and follows Go convention. Included for completeness only - no patch recommended.

---

### SMELL-5: Sync-conflict files in working directory (Severity: Info/Hygiene)

The working directory contains several Syncthing conflict files:
- `.sync-conflict-20260215-173103-VIHEVVL.env`
- `gemini/client_test.sync-conflict-20260215-173043-VIHEVVL.go`
- `gemini/types.sync-conflict-20260215-173222-VIHEVVL.go`
- `go.sync-conflict-20260215-173017-VIHEVVL.sum`

These are untracked (correctly not committed) but should be cleaned up to avoid confusion. The `.gitignore` does not have a pattern for `*.sync-conflict-*` files.

**Patch (.gitignore):**
```diff
--- a/.gitignore
+++ b/.gitignore
@@ -1,5 +1,8 @@
 # Secrets
 .env
 .env.local
 .env.*

+# Syncthing conflict files
+*.sync-conflict-*
+
 # OS files
```

---

### SMELL-6: `run()` in main.go doesn't validate prompt content (Severity: Very Low)

**File:** `cmd/gemini/main.go:51-53`

The CLI checks `len(args) == 0` but doesn't check if all args are whitespace-only. `gemini " " " "` would send a prompt of `"   "` to the API. This is very minor since BUG-2's fix at the library level would catch it.

---

### OBSERVATION-1: Test coverage is thorough (Positive)

The test suite covers:
- Constructor validation (empty key, HTTP URL, whitespace key, empty model, invalid model names, valid model names)
- Request building (URL, headers, body, role, tools)
- Generate options (maxTokens, temperature, Google Search)
- Response parsing (success, no candidates, no parts, multiple parts, multiple candidates)
- Error handling (HTTP errors, doer errors, invalid JSON, empty response, oversized response, error truncation)
- Edge cases (zero temperature, GetBody for retry, context propagation)
- Serialization (omitempty, temperature zero pointer)
- Config loading (defaults, overrides, panic on missing required)

This is excellent coverage for a project of this size.

---

### OBSERVATION-2: Security posture is good (Positive)

- API key in header (not URL query parameter)
- HTTPS enforcement on base URL
- Model name regex prevents path traversal
- Response body size limiting (10MB)
- Error body truncation prevents info leakage
- No credentials in committed code (placeholder in .env)

---

## Scoring Breakdown

| Category | Max Points | Score | Notes |
|---|---|---|---|
| **Correctness** | 25 | 22 | BUG-1 (untrimmed key) and BUG-2 (empty prompt) are real but low-severity |
| **Security** | 20 | 19 | Strong: HTTPS enforcement, header auth, input validation, response limits. Minor: untrimmed key could leak whitespace |
| **Code Quality** | 15 | 13 | Clean idiomatic Go, good separation of concerns. Minor: magic number duplication, trailing slash handling |
| **Test Coverage** | 20 | 18 | Comprehensive suite with edge cases. Missing: empty prompt test, trimmed key test, trailing-slash base URL test |
| **Documentation** | 10 | 5 | Godoc comments present on exported types/functions. No package-level doc on gemini/client.go. CHANGELOG is well-maintained. No README.md. |
| **Project Hygiene** | 10 | 5 | Sync-conflict files present. VERSION.chassis semantics unclear. .gitignore could be tighter. |

**TOTAL: 82/100**

---

## Summary of Recommended Fixes (Priority Order)

1. **BUG-1**: Trim API key before storing (prevents whitespace auth failures)
2. **BUG-2**: Validate non-empty prompt in `Generate()` (prevents wasted API calls)
3. **SMELL-2**: Trim trailing slash from base URL (prevents double-slash URLs)
4. **SMELL-1**: Export default constants to prevent drift between library and CLI
5. **SMELL-5**: Add `*.sync-conflict-*` to `.gitignore` and clean up conflict files
