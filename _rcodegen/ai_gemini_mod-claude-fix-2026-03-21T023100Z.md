Date Created: 2026-03-21T02:31:00Z
TOTAL_SCORE: 82/100

# ai_gemini_mod Code Audit Report

**Agent:** Claude:Opus 4.6
**Scope:** All Go source files (gemini/client.go, gemini/types.go, gemini/client_test.go, cmd/gemini/main.go, cmd/gemini/main_test.go, go.mod)

---

## Summary

The codebase is well-structured with strong security practices (HTTPS enforcement, model name validation, response size limits, error body truncation, API key validation) and good test coverage (30+ tests). The functional options pattern is cleanly implemented, context propagation is correct, and the `Response.Text()` method is nil-safe.

The primary issues center around the **dual-path request strategy** (addon vs. HTTP) being structurally broken in the production CLI binary, a **silently discarded error** during initialization, and a **misleading timeout calculation**.

---

## Issues Found

### ISSUE 1: Addon Code Path Is Dead in Production (HIGH)

**Files:** `gemini/client.go:98-115`, `cmd/gemini/main.go:71-79`
**Severity:** High (architectural correctness)

The `llmClient` (chassis-go-addons/llm) is only initialized when `c.doer` is an `*http.Client` (line 102). However, the production CLI binary always injects a `call.Client` via `WithDoer(caller)` at `main.go:79`. Since `call.Client` is not `*http.Client`, the type assertion fails and `c.llmClient` remains nil. All production requests go through `generateViaHTTP`.

Additionally, `GoogleSearch` defaults to `true` in the CLI config (`main.go:36`), which forces the HTTP path even if the addon were initialized (`client.go:178`).

The comment at `client.go:98-100` ("This covers normal production use") is incorrect — the addon path is never exercised in production.

**Impact:** The addon integration added in v1.3.6 provides no value in the production binary. Two code paths must be maintained but only one is ever used. Bugs in `generateViaAddon` would go undetected.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -96,10 +96,11 @@

-	// Build an llm addon client when the doer is a standard *http.Client.
-	// This covers normal production use. When a custom Doer (e.g. a test
-	// mock) is injected, the addon path is skipped and doRequest is used
-	// as a fallback.
+	// Build an llm addon client when the doer is a standard *http.Client
+	// AND no custom Doer has been injected. NOTE: When a custom Doer is
+	// injected (including call.Client in the CLI), this block is skipped
+	// and generateViaHTTP is used exclusively. The addon path currently
+	// only activates in direct library usage with default HTTP client.
 	if hc, ok := c.doer.(*http.Client); ok {
```

**Recommended architectural fix:** Either (a) make the addon client work with any `Doer`, not just `*http.Client`, or (b) remove the addon integration from the library and keep the single HTTP path, or (c) stop injecting `WithDoer` in `main.go` and let the addon client handle retries/timeout internally.

---

### ISSUE 2: Silent Swallowing of llm.NewClient Error (HIGH)

**File:** `gemini/client.go:110-114`
**Severity:** High (observability/correctness)

When `llm.NewClient` returns an error, it is silently discarded. The client falls back to `generateViaHTTP` with no signal to the caller. A misconfiguration (bad model name, incompatible options) would produce degraded behavior instead of a clear startup failure.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -107,10 +107,9 @@
 			BaseURL:  addonBaseURL(c.baseURL, c.model),
 			Doer:     hc,
 		})
-		if err == nil {
-			c.llmClient = addonClient
+		if err != nil {
+			return nil, fmt.Errorf("gemini: init llm addon client: %w", err)
 		}
-		// If addon creation fails (shouldn't normally), fall back silently
-		// to doRequest.
+		c.llmClient = addonClient
 	}
```

**Alternative (if silent fallback is desired by design):** At minimum, log the error so operators can detect degraded initialization.

---

### ISSUE 3: Context Timeout Arithmetic Is Confusing and Likely Overcounted (MEDIUM)

**File:** `cmd/gemini/main.go:23-27, 85`
**Severity:** Medium (correctness/maintainability)

```go
const (
    retryAttempts      = 3
    retryTotalAttempts = retryAttempts + 1 // = 4
)
// ...
ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalAttempts+1))
// Effective: 30s * 5 = 150s
```

`retryTotalAttempts` already accounts for the initial attempt (3 retries + 1 = 4 attempts). The `+1` in the timeout formula adds a 5th multiplier. The comment says "+1 provides buffer for backoff gaps" but the extra 30s of budget far exceeds the actual backoff time (500ms + 1s + 2s = 3.5s with exponential backoff). This creates a 150-second timeout for a CLI tool, which is surprising.

```diff
--- a/cmd/gemini/main.go
+++ b/cmd/gemini/main.go
@@ -23,6 +23,9 @@
 const (
 	retryAttempts      = 3
 	retryBaseDelay     = 500 * time.Millisecond
 	retryTotalAttempts = retryAttempts + 1 // initial attempt + retries
+	// retryBackoffBudget is the maximum total backoff sleep across all retries
+	// (500ms + 1s + 2s = 3.5s with exponential backoff). Rounded up to 5s.
+	retryBackoffBudget = 5 * time.Second
 )

@@ -84,2 +87,2 @@
-	// Allow enough time for all retry attempts; +1 provides buffer for backoff gaps.
-	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalAttempts+1))
+	// Allow enough time for all retry attempts plus backoff sleep between them.
+	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(retryTotalAttempts)+retryBackoffBudget)
```

This changes the timeout from 150s to 125s (4*30s + 5s), which is still generous but mathematically justified.

---

### ISSUE 4: Model Validation Regex Allows Malformed URL Segments (LOW)

**File:** `gemini/client.go:29`
**Severity:** Low (defense in depth)

The regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` permits:
- Consecutive slashes: `model//name` -> URL becomes `.../model//name:generateContent`
- Trailing slash: `model/` -> URL becomes `.../model/:generateContent`
- Leading-dot after first char: `a.` is valid

These would produce malformed URLs that the API would reject, but they bypass the validation that is explicitly intended to catch such issues.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -28,2 +28,3 @@
 // validModel matches model names: alphanumeric, dots, hyphens, underscores, slashes.
-var validModel = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)
+// Disallows consecutive slashes, leading/trailing dots or slashes.
+var validModel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]|/(?!/))*[a-zA-Z0-9]$`)
```

Note: This stricter regex requires the model name to be at least 2 characters and end with alphanumeric. Single-character model names would need separate handling if needed.

---

### ISSUE 5: Unused Parameter in addonBaseURL (LOW)

**File:** `gemini/client.go:125`
**Severity:** Low (code smell)

```go
func addonBaseURL(baseURL, _ string) string {
```

The second parameter (model) is accepted but explicitly ignored. This creates a misleading function signature. If the parameter isn't needed, the function should take only one argument. If it's a placeholder for future use, it should be documented as such.

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -120,7 +120,7 @@
 // addonBaseURL converts the gemini module's per-model baseURL (which already
 // includes "/models") into the base URL the addon expects (without "/models").
-// The addon constructs URLs as: baseURL + "/models/" + model + ":generateContent"
-// Our baseURL already looks like ".../v1beta/models", so we strip the trailing
-// "/models" to let the addon reconstruct it.
-func addonBaseURL(baseURL, _ string) string {
+// Our baseURL looks like ".../v1beta/models"; the addon reconstructs it as
+// baseURL + "/models/" + model + ":generateContent", so we strip "/models".
+func addonBaseURL(baseURL string) string {
 	if strings.HasSuffix(baseURL, "/models") {
```

Corresponding call site update:

```diff
--- a/gemini/client.go
+++ b/gemini/client.go
@@ -107,1 +107,1 @@
-			BaseURL:  addonBaseURL(c.baseURL, c.model),
+			BaseURL:  addonBaseURL(c.baseURL),
```

---

### ISSUE 6: Potentially Redundant os.Exit After registry.ShutdownCLI (LOW)

**File:** `cmd/gemini/main.go:52-53`
**Severity:** Low (clarity)

```go
registry.ShutdownCLI(1)
os.Exit(1)
```

If `ShutdownCLI(1)` internally calls `os.Exit(1)`, the second `os.Exit` is unreachable dead code. If it doesn't, the double-call pattern is confusing because `ShutdownCLI` takes an exit code parameter suggesting it handles exit itself.

```diff
--- a/cmd/gemini/main.go
+++ b/cmd/gemini/main.go
@@ -50,4 +50,3 @@
 	if err := run(os.Args[1:]); err != nil {
 		fmt.Fprintf(os.Stderr, "error: %v\n", err)
-		registry.ShutdownCLI(1)
-		os.Exit(1)
+		registry.ShutdownCLI(1) // handles cleanup and exit
 	}
```

**Note:** This depends on `registry.ShutdownCLI` behavior. If it does NOT call `os.Exit`, keep the explicit `os.Exit(1)` but document the contract.

---

## Scoring Breakdown

| Category | Points | Deductions | Notes |
|----------|--------|------------|-------|
| **Correctness** | 25/30 | -5 | Dead addon path in production, silent error swallowing |
| **Security** | 18/20 | -2 | Model regex edge cases allow malformed URLs |
| **Error Handling** | 13/15 | -2 | Silent discard of `llm.NewClient` error |
| **Code Quality** | 12/15 | -3 | Unused parameter, confusing timeout arithmetic, redundant exit |
| **Test Coverage** | 14/15 | -1 | No tests for addon path (`generateViaAddon`); addon never reachable in tests either since tests use mockDoer |
| **Documentation** | 5/5 | 0 | README is comprehensive, comments are clear |

**TOTAL: 82/100**

---

## Positive Observations

- **Security-first design:** HTTPS-only enforcement, header-based API key (not URL query), model name regex validation, 10MB response size limit, 1KB error body truncation
- **Strong test suite:** 30+ test cases covering edge cases, validation bounds, nil receivers, serialization, context propagation, retry support (GetBody)
- **Clean API design:** Functional options for both `Client` construction and per-request configuration; `Doer` interface for dependency injection
- **Proper resource management:** `defer resp.Body.Close()`, `io.LimitReader` for bounded reads
- **Nil-safe helpers:** `Response.Text()` handles nil receiver, empty candidates, empty parts
- **Good use of chassis framework:** Config loading via struct tags, structured logging, registry lifecycle management
