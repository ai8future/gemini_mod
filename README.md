# ai_gemini_mod

A Go library and CLI tool for the Google Gemini generative AI API.

## Overview

This project provides two components:

- **`gemini/` package** — A reusable Go library wrapping the Gemini `generateContent` REST endpoint. It features a clean functional-options API, injectable HTTP transport via the `Doer` interface, and built-in input validation and security hardening.
- **`cmd/gemini/` binary** — A CLI wrapper that reads configuration from environment variables, applies retry/timeout policies via [chassis-go](https://github.com/ai8future/chassis-go), and prints the JSON response to stdout.

## Installation

```sh
go get ai_gemini_mod/gemini
```

To build the CLI:

```sh
go build -o gemini ./cmd/gemini
```

## CLI Usage

```sh
export GEMINI_API_KEY="your-api-key"
gemini What is the capital of France?
```

All arguments after the binary name are joined as the prompt. The full API response is printed as pretty-printed JSON.

### Environment Variables

| Variable | Type | Default | Required | Description |
|---|---|---|---|---|
| `GEMINI_API_KEY` | string | — | yes | Google AI Studio API key |
| `GEMINI_MODEL` | string | `gemini-3-pro-preview` | no | Model identifier |
| `GEMINI_MAX_TOKENS` | int | `32000` | no | Max output tokens (1–1,000,000) |
| `GEMINI_TEMPERATURE` | float64 | `1.0` | no | Sampling temperature (0.0–2.0) |
| `GEMINI_TIMEOUT` | duration | `30s` | no | Per-attempt HTTP timeout |
| `GEMINI_GOOGLE_SEARCH` | bool | `true` | no | Enable Google Search grounding |
| `LOG_LEVEL` | string | `error` | no | Logging verbosity (debug/info/error) |

## Library Usage

### Basic

```go
package main

import (
    "context"
    "fmt"
    "log"

    "ai_gemini_mod/gemini"
)

func main() {
    client, err := gemini.New("your-api-key")
    if err != nil {
        log.Fatal(err)
    }

    resp, err := client.Generate(context.Background(), "Explain quantum computing")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Text())
}
```

### With Options

```go
client, err := gemini.New("your-api-key",
    gemini.WithModel("gemini-2.5-flash"),
    gemini.WithTimeout(60 * time.Second),
)
if err != nil {
    log.Fatal(err)
}

resp, err := client.Generate(ctx, "Summarize this article",
    gemini.WithMaxTokens(8000),
    gemini.WithTemperature(0.3),
    gemini.WithGoogleSearch(),
)
```

### Custom HTTP Transport

The `Doer` interface accepts any type with a `Do(*http.Request) (*http.Response, error)` method, making it easy to plug in retry middleware, instrumented transports, or test mocks:

```go
caller := call.New(
    call.WithTimeout(30 * time.Second),
    call.WithRetry(3, 500 * time.Millisecond),
)

client, err := gemini.New("your-api-key",
    gemini.WithDoer(caller),
)
```

## API

### Client Construction

| Function | Description |
|---|---|
| `New(apiKey string, opts ...Option) (*Client, error)` | Create a client. Validates key, model, and base URL. |
| `WithModel(model string) Option` | Override the default model (`gemini-3-pro-preview`). |
| `WithDoer(d Doer) Option` | Inject a custom HTTP executor. |
| `WithBaseURL(url string) Option` | Override the API base URL (must be HTTPS). |
| `WithTimeout(d time.Duration) Option` | Set timeout on the default HTTP client. Ignored when `WithDoer` is used. |

### Generation

| Function | Description |
|---|---|
| `Generate(ctx context.Context, prompt string, opts ...GenerateOption) (*Response, error)` | Send a prompt and return the parsed response. |
| `WithMaxTokens(n int) GenerateOption` | Set max output tokens (1–1,000,000). Default: 32,000. |
| `WithTemperature(t float64) GenerateOption` | Set sampling temperature (0.0–2.0). Default: 1.0. |
| `WithGoogleSearch() GenerateOption` | Enable grounding with Google Search. |

### Response

| Method | Description |
|---|---|
| `(*Response).Text() string` | Concatenated text from all parts of the first candidate. Nil-safe. |

The `Response` struct also exposes `Candidates` (with finish reason and safety ratings) and `UsageMetadata` (prompt, candidate, and total token counts).

## Security

- API key transmitted via `x-goog-api-key` header (not query parameter)
- Base URL restricted to HTTPS
- Model names validated against a strict regex to prevent URL path injection
- Response bodies capped at 10 MB to prevent memory exhaustion
- Error bodies truncated at 1 KB in error messages
- `.env` file is gitignored

## Testing

```sh
go test ./...
```

Tests use a mock `Doer` implementation — no real API calls are made. Coverage includes client construction, request building, input validation, HTTP error handling, response parsing, edge cases (nil response, empty candidates, oversized bodies), and context propagation.

## Dependencies

| Package | Purpose |
|---|---|
| [chassis-go/v6](https://github.com/ai8future/chassis-go) | Config loading, structured logging, HTTP retry/timeout |

## Project Structure

```
ai_gemini_mod/
├── gemini/
│   ├── types.go         # Request/response types and Text() helper
│   ├── client.go        # Client, Doer interface, New(), Generate()
│   └── client_test.go   # Unit tests (mock-based, ~30 tests)
├── cmd/gemini/
│   ├── main.go          # CLI entry point and Config struct
│   └── main_test.go     # Config loading tests
├── go.mod
├── VERSION              # Current version
├── CHANGELOG.md         # Version history
└── .env                 # Local env vars (gitignored)
```

## License

Private repository. All rights reserved.
