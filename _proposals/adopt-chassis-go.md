# Proposal: Adopt chassis-go for gemini_test

## Current State

`gemini_test` is a simple CLI tool that sends prompts to the Gemini 3 Pro API with Google Search grounding enabled. It currently:

- Loads `.env` via `godotenv`
- Reads `GEMINI_API_KEY` from env manually with `os.Getenv`
- Makes a raw `http.Client` POST to the Gemini API
- Pretty-prints the full JSON response
- Has no structured logging, retry logic, or error resilience

## Proposed Changes

Since this is a CLI tool (not a service), the relevant chassis-go packages are **config**, **logz**, **call**, and **testkit**. The service-oriented packages (httpkit, grpckit, lifecycle, health) don't apply here.

---

### 1. Replace `godotenv` + manual `os.Getenv` with `config.MustLoad`

**Why**: Eliminates the `godotenv` dependency entirely and provides typed, validated configuration with fail-fast semantics.

**Before**:
```go
godotenv.Load()
apiKey := os.Getenv("GEMINI_API_KEY")
if apiKey == "" {
    fmt.Fprintln(os.Stderr, "Error: GEMINI_API_KEY environment variable not set")
    os.Exit(1)
}
```

**After**:
```go
type Config struct {
    APIKey      string        `env:"GEMINI_API_KEY"`
    Model       string        `env:"GEMINI_MODEL" default:"gemini-3-pro-preview"`
    MaxTokens   int           `env:"GEMINI_MAX_TOKENS" default:"32000"`
    Temperature float64       `env:"GEMINI_TEMPERATURE" default:"1.0"`
    Timeout     time.Duration `env:"GEMINI_TIMEOUT" default:"30s"`
}

cfg := config.MustLoad[Config]()
```

**Notes**:
- chassis-go v1.0.3+ supports `float64`, so temperature is a proper config field.
- `godotenv` is removed entirely. Env vars are set in the shell.
- This also opens the door to making the model name, max tokens, and timeout configurable via env vars rather than hardcoded constants.

---

### 2. Replace raw `http.Client` with `call.New` for resilient API calls

**Why**: The Gemini API can return transient 5xx errors or experience network issues. `call.Client` adds automatic retries with exponential backoff and optional circuit breaking.

**Before**:
```go
client := &http.Client{}
resp, err := client.Do(req)
```

**After**:
```go
client := call.New(
    call.WithTimeout(cfg.Timeout),
    call.WithRetry(3, 500*time.Millisecond),
)
```

**Notes**:
- Circuit breaking is unnecessary for a CLI tool making a single request, so we skip `WithCircuitBreaker`.
- Retry with backoff is valuable since LLM API endpoints frequently return 503/529 under load.
- The timeout becomes configurable via the `GEMINI_TIMEOUT` env var.

---

### 3. Add structured logging with `logz`

**Why**: Replaces `fmt.Fprintln(os.Stderr, ...)` with structured JSON logging, useful for debugging API issues and piping output through log aggregation.

**Before**:
```go
fmt.Fprintln(os.Stderr, "Error: GEMINI_API_KEY environment variable not set")
fmt.Fprintf(os.Stderr, "Error making request: %v\n", err)
```

**After**:
```go
logger := logz.New(cfg.LogLevel)
logger.Error("request failed", "error", err)
logger.Info("request complete", "model", cfg.Model, "status", resp.StatusCode)
```

**Notes**:
- Add a `LOG_LEVEL` config field (default: `"error"` for a CLI tool so normal usage is quiet).
- Structured logging makes it easy to add `--verbose` behavior by setting `LOG_LEVEL=debug`.
- The API response itself should still go to stdout as raw/pretty JSON, keeping logging and output separate.

---

### 4. Extract just the generated text from the response

**Why**: Currently the tool dumps the entire API response JSON, which includes metadata, safety ratings, grounding chunks, etc. For CLI usability, extracting the text content is more practical.

**This is independent of chassis-go** but is a natural improvement when restructuring the code.

**Proposed behavior**:
- Default: print only the generated text to stdout
- With `LOG_LEVEL=debug`: also log the full response JSON

---

### 5. Add tests using `testkit`

**Why**: The codebase currently has no tests. `testkit` provides test helpers that pair with chassis config and logging.

**What to test**:
- Config loading with `testkit.SetEnv` + `config.MustLoad`
- Request body construction (JSON marshaling)
- Response parsing (extract text from Gemini response JSON)

```go
func TestConfigLoading(t *testing.T) {
    testkit.SetEnv(t, map[string]string{
        "GEMINI_API_KEY": "test-key",
        "GEMINI_MODEL":   "gemini-3-pro-preview",
    })
    cfg := config.MustLoad[Config]()
    if cfg.APIKey != "test-key" {
        t.Errorf("expected test-key, got %s", cfg.APIKey)
    }
}
```

---

## Dependency Changes

| Remove | Add |
|--------|-----|
| `github.com/joho/godotenv` | `github.com/ai8future/chassis-go v1.0.3+` |

`godotenv` is fully removed. Env vars are managed by the shell (profile export, inline, or direnv).

---

## Summary of Changes

| File | Change |
|------|--------|
| `main.go` | Replace godotenv/os.Getenv with `config.MustLoad`, replace `http.Client` with `call.New`, add `logz` logging, extract response text |
| `go.mod` | Add `chassis-go`, optionally remove `godotenv` |
| `main_test.go` | New file with tests using `testkit` |

## What We Skip

These chassis-go packages are **not applicable** to a CLI tool:

- **httpkit** - no HTTP server to middleware
- **grpckit** - no gRPC server
- **lifecycle** - no long-running components to orchestrate
- **health** - no health endpoint needed

## Resolved Decisions

1. **Drop `.env` file support.** Remove `godotenv` entirely. Env vars are set via shell profile, inline invocation (`GEMINI_API_KEY=xxx gemini "prompt"`), or direnv. `config.MustLoad` reads `os.Getenv` directly and panics with a clear message if required vars are missing.
2. **`float64` supported in chassis-go v1.0.3.** Temperature is now a proper `float64` config field -- no workarounds needed. Pin to `v1.0.3+`.
3. **Default output is full JSON.** The tool continues to pretty-print the complete API response. Use `LOG_LEVEL=debug` for additional diagnostic logging to stderr.
