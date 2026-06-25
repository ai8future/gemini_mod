# PRODUCT.md -- ai_gemini_mod

## What This Product Is

ai_gemini_mod is a Go-based client library and command-line interface for Google's Gemini generative AI API. It serves as the organization's standardized, production-grade integration point for all Gemini-powered AI capabilities. It is one of three provider-specific client modules in the ai_suite ecosystem (alongside ai_claude_mod for Anthropic Claude and ai_openai_mod for OpenAI), each providing a uniform, hardened interface to a major LLM provider.

## Why This Product Exists

### Strategic Context

The product exists to solve a specific organizational problem: enabling reliable, secure, and operationally consistent access to Google Gemini models across multiple downstream systems. Rather than having each consuming application (e.g., the "airborne" multi-provider gateway, the "dispatch" tool orchestration runtime, or ad-hoc developer scripts) implement their own Gemini API integration, ai_gemini_mod centralizes that concern into a single, tested, versioned module.

This serves several business goals:

1. **Reduce integration risk.** A single, well-tested Gemini client eliminates duplicated API integration code across projects. Every application that needs Gemini talks through this module, which means bugs, security issues, and API changes are fixed in one place.

2. **Enable the multi-provider AI gateway.** The ai_suite ecosystem includes "airborne," a unified gRPC gateway that routes LLM requests across many providers. ai_gemini_mod is the Gemini leg of that gateway. Without a clean, reusable client library, the gateway would need to embed raw HTTP logic for Gemini inline, making it harder to maintain and test.

3. **Standardize operational patterns.** By building on the shared "chassis-go" infrastructure framework (currently chassis-go/v11, for config, logging, HTTP retry/timeout, error classification, and CLI lifecycle), ai_gemini_mod ensures that Gemini interactions follow the same operational patterns as every other service and CLI tool in the organization. This means consistent logging formats, consistent error categories, consistent retry behavior, and consistent environment-variable-driven configuration.

4. **Provide a standalone CLI for rapid experimentation.** Beyond library use, the CLI tool lets developers and operators quickly test prompts against Gemini from the terminal without writing any code. This accelerates prototyping, debugging, and evaluation of Gemini models.

### Business Problem Being Solved

At the core, ai_gemini_mod solves the problem of **making LLM API calls reliable and secure in production environments**. Raw HTTP calls to an LLM API are fragile: they lack retries, have no timeout discipline, risk leaking API keys in URLs, and provide no structured error handling. This module wraps all of that complexity into a clean interface that downstream consumers can trust.

## What the Product Does (Functional Capabilities)

### 1. Gemini API Client Library (the `gemini/` package)

The primary deliverable is a reusable Go library that any Go application can import to communicate with Google's Gemini `generateContent` REST endpoint.

**Core capabilities:**

- **Prompt submission.** Accepts a text prompt and sends it to a configurable Gemini model (defaulting to `gemini-3-pro-preview`). Returns the model's generated text response along with metadata (token usage, safety ratings, finish reason).

- **Google Search grounding.** Supports enabling Google Search as a "tool" for the Gemini model, allowing the model to ground its responses in real-time web search results. This is a key differentiator for Gemini and is enabled by default in the CLI. This capability is critical for use cases where responses must reflect current information rather than training-data-only knowledge.

- **Generation parameter control.** Callers can configure:
  - **Max output tokens** (1 to 1,000,000) -- controls response length and cost
  - **Temperature** (0.0 to 2.0) -- controls creativity vs. determinism
  - **Model selection** -- switch between Gemini model variants (e.g., gemini-3-pro-preview, gemini-2.5-flash) depending on cost/quality tradeoffs

- **Custom HTTP transport injection.** The `Doer` interface (`Do(*http.Request) (*http.Response, error)`) allows callers to inject their own HTTP client, enabling:
  - Retry middleware with exponential backoff (via chassis-go's `call` package)
  - Circuit breakers for protecting against cascading failures
  - Instrumented transports for observability (OpenTelemetry integration)
  - Test mocks for unit testing without real API calls

- **Single native HTTP execution path.** Every call to `Generate` builds a `Request`, then routes through `generateViaHTTP` → `doRequest`, which marshals JSON and POSTs to Gemini's native `:generateContent` endpoint via the injected `Doer`. There is exactly one code path; all requests (with or without Google Search) go through it.

  **Why this matters:** An earlier iteration (v1.3.6) routed standard requests through a shared `chassis-go-addons/llm` abstraction and reserved a fallback HTTP path only for Google Search grounding and Doer mocks. That addon was removed in v1.3.13, collapsing the design back to a single hand-rolled `generateContent` implementation. Anyone modifying request/response handling should edit `gemini/client.go` and `gemini/types.go` directly -- there is no longer an external abstraction layer to coordinate with.

### 2. Command-Line Interface (the `cmd/gemini/` binary)

The CLI is a thin wrapper around the library, designed for interactive use and scripting.

**Capabilities:**

- **Environment-driven configuration.** All settings are controlled via environment variables (`GEMINI_API_KEY`, `GEMINI_MODEL`, `GEMINI_MAX_TOKENS`, `GEMINI_TEMPERATURE`, `GEMINI_TIMEOUT`, `GEMINI_GOOGLE_SEARCH`, `LOG_LEVEL`), with sensible defaults. The only required variable is the API key.

- **Prompt from command-line arguments.** All arguments after the binary name are joined into a single prompt string (e.g., `gemini What is the capital of France?`).

- **JSON output.** The full API response is pretty-printed as JSON to stdout, preserving all metadata (candidates, token usage, safety ratings) for downstream processing or piping into other tools.

- **Resilient HTTP with automatic retries.** The CLI configures 3 retry attempts with 500ms base delay and exponential backoff. The overall context timeout is calculated to accommodate all retry attempts. This makes the CLI robust against transient Gemini API failures (503s, rate limits, network blips).

- **Structured logging.** Uses the chassis-go `logz` package for structured logging to stderr. Default log level is `error` (quiet for normal use), but can be set to `debug` for full diagnostic output.

- **Environment detection.** Uses chassis-go's `deploy.Discover` to detect the runtime environment and load `.env` files, supporting local development workflows.

- **CLI lifecycle management.** Integrates with chassis-go's `registry` package for standardized CLI initialization and shutdown, including clean exit codes.

## Business Logic

### Input Validation and Safety

The module enforces strict validation on all inputs:

- **API key**: Must not be empty or whitespace-only. Prevents accidental unauthenticated calls.
- **Model name**: Validated against a regex (`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`) to prevent URL path injection attacks. This is a security measure: since the model name is interpolated into the API URL, a malicious model name like `../../evil` could redirect requests to an attacker-controlled endpoint.
- **Base URL**: Must use HTTPS. Prevents accidental transmission of API keys over unencrypted connections.
- **Temperature**: Bounded between 0.0 and 2.0 (Gemini's valid range). Prevents wasted API calls that would be rejected server-side.
- **Max tokens**: Bounded between 1 and 1,000,000. Prevents both zero-token requests (useless) and excessively large token requests (cost risk).

### Security Hardening

- **API key in headers, not URL.** The API key is transmitted via the `x-goog-api-key` HTTP header rather than as a URL query parameter. This prevents key leakage in server logs, browser history, and network monitoring tools.
- **Response size limit.** Response bodies are capped at 10 MB to prevent memory exhaustion from malformed or malicious API responses.
- **Error body truncation.** Error response bodies are truncated to 1 KB in error messages to prevent sensitive information from leaking into logs.
- **Secret management.** The `.env` file (which may contain API keys) is gitignored to prevent accidental credential commits.

### Error Classification

The module uses chassis-go's typed error system to categorize errors:

- **Validation errors** (via `chassiserrors.ValidationError`): Invalid API key, model name, base URL, temperature, or max tokens. These are caller mistakes.
- **Dependency errors** (via `chassiserrors.DependencyError`): HTTP failures, network errors, non-2xx status codes, response read failures. These are external system failures.

This classification enables downstream systems (like the airborne gateway) to make intelligent decisions about error handling -- e.g., retrying dependency errors but immediately rejecting validation errors.

### Retry and Resilience Strategy

The CLI configures a specific retry policy:

- 3 retry attempts (4 total attempts including the initial request)
- 500ms base delay with exponential backoff
- Per-attempt timeout configurable via `GEMINI_TIMEOUT` (default 30s)
- Overall context timeout calculated as `timeout * (totalAttempts + 1)` to provide buffer for backoff gaps

This policy is specifically tuned for LLM API interactions, which are known for transient failures under load (503, 429 rate limits).

### Request Replay Support

The library sets `req.GetBody` on every HTTP request, which allows retry middleware to replay the request body on subsequent attempts. Without this, retries would send empty bodies because the original body reader is consumed on the first attempt.

## How This Fits Into the Larger Ecosystem

ai_gemini_mod is one component in a layered AI infrastructure:

1. **Provider client modules** (ai_gemini_mod, ai_claude_mod, ai_openai_mod) -- Hardened, tested wrappers for individual LLM provider APIs, all built on the shared chassis-go framework.
2. **Multi-provider gateway** (airborne) -- A gRPC gateway that routes requests across providers, using the client modules as its backend integrations.
3. **Tool orchestration** (dispatch) -- A runtime that can invoke LLM-powered tools, potentially routed through airborne and ultimately through modules like ai_gemini_mod.
4. **Pricing** (pricing_db) -- A cost tracking system that needs to know token counts (which ai_gemini_mod exposes via `UsageMetadata`).

Note: a shared `chassis-go-addons/llm` abstraction was briefly used (v1.3.6) but removed (v1.3.13). Today each provider module owns its native HTTP implementation; standardization comes from the common chassis-go framework (config, logging, retry, typed errors), not from a shared request/response abstraction.

The module's token usage metadata (`PromptTokenCount`, `CandidatesTokenCount`, `TotalTokenCount`) is particularly important in this ecosystem, as it feeds into cost tracking and billing systems.

## Product Evolution

The changelog reveals a deliberate product evolution:

- **v1.0.0**: Initial release with basic Gemini API wrapping and CLI.
- **v1.1.0-v1.3.8**: Continuous upgrades to track the rapidly evolving chassis-go framework (v4 through v10, now v11), indicating this module is part of an actively maintained infrastructure platform.
- **v1.2.0**: Major security hardening pass -- API key leak remediation, model name validation, input bounds checking, testability refactoring. This suggests the product matured from a prototype to a production-grade component.
- **v1.3.6**: Brief integration with the shared LLM abstraction layer (chassis-go-addons/llm) for standard requests, with a fallback HTTP path for Google Search and mocks.
- **v1.3.9**: Adoption of chassis-go's typed error system, improving interoperability with downstream error-handling infrastructure.
- **v1.3.10-v1.3.11**: GO-BEST-PRACTICES conformance -- cross-platform Makefile (static `CGO_ENABLED=0` builds), launcher script, and an expanded test suite.
- **v1.3.12**: Embedded VERSION via `appversion.go` and adoption of chassis's `SetAppVersion` pattern.
- **v1.3.13**: Removal of the `chassis-go-addons/llm` dependency -- all Gemini calls reverted to the native `generateContent` implementation, collapsing the dual-path design back to a single hand-rolled client.
- **v1.3.14**: Dependency and config refresh (current chassis-go/v11).

The product is in a mature state, with security hardening complete, comprehensive mock-based test coverage (50 unit tests across the `gemini/` and `cmd/gemini/` packages), and standardization anchored on the shared chassis-go framework.

## Key Business Decisions Embedded in the Code

1. **Google Search grounding is ON by default** in the CLI (`GEMINI_GOOGLE_SEARCH` defaults to `true`). This reflects a business decision that Gemini's primary value proposition is access to current information via search grounding, not just static model knowledge.

2. **Default model is `gemini-3-pro-preview`**, the latest and most capable Gemini model. This shows a preference for capability over cost in the default configuration.

3. **Default max tokens is 32,000**, which is generous. This suggests the primary use cases involve longer-form content generation rather than short Q&A.

4. **Default temperature is 1.0**, which is moderate creativity. This is Gemini's standard default and suggests general-purpose use rather than highly deterministic or highly creative applications.

5. **Full JSON response output** rather than extracted text. The CLI outputs the complete API response including metadata, safety ratings, and token counts. This prioritizes transparency and debuggability over user-friendliness, consistent with the tool being used by developers and operators rather than end users.

6. **Private repository.** The product is proprietary and not open-sourced, indicating it is an internal infrastructure component rather than a community offering.

## How to Think About Code Changes

- **This repo owns one thing: the Gemini `generateContent` integration.** Provider-agnostic routing, fan-out across providers, and request brokering belong in the `airborne` gateway, not here. Tool orchestration belongs in `dispatch`. Cost math belongs in `pricing_db`. This module's job is to turn a validated prompt into a typed, hardened HTTP call to Gemini and parse the result.
- **All request/response logic lives in `gemini/client.go` and `gemini/types.go`.** There is a single execution path (`Generate` → `generateViaHTTP` → `doRequest`); do not reintroduce branching execution paths or an external request abstraction without a deliberate decision (see the v1.3.6/v1.3.13 history above).
- **Never weaken the security invariants.** API key stays in the `x-goog-api-key` header (never the URL); base URL stays HTTPS-only; model names stay regex-validated before URL interpolation; response bodies stay capped at 10 MB; error bodies stay truncated to 1 KB. These are load-bearing for production safety, not stylistic.
- **Keep the `Doer` seam intact.** It is how the CLI injects chassis-go's retrying `call.Client` and how tests inject mocks (no real API calls in the suite). Changes to HTTP handling must continue to flow through `Doer.Do` and must keep setting `req.GetBody` so retry middleware can replay the body.
- **chassis-go is the standardization layer.** Use its `config`, `logz`, `call`, `deploy`, `registry`, and typed `errors` factories rather than hand-rolling equivalents. The CLI gates on `chassis.RequireMajor(11)`; a chassis major bump is a coordinated, intentional change.
- **Versioning/CHANGELOG discipline** is governed by `AGENTS.md` (bump `VERSION`, annotate `CHANGELOG.md`, keep `VERSION.chassis` in sync with the chassis major in `go.mod`). The `gemini/` library and `cmd/gemini/` CLI must stay decoupled: the library has no chassis dependency beyond the typed `errors` package; chassis lifecycle (config, deploy, registry) lives only in the CLI.

## Current State / Status

- **Version:** 1.3.14 (see `VERSION`); chassis-go pinned at v11.1.8 (see `VERSION.chassis` and `go.mod`, which uses a local `replace` directive to `../../chassis_suite/chassis-go`).
- **Built and in use:** the `gemini/` client library (prompt submission, Google Search grounding, generation-parameter control, input validation, security hardening, typed errors) and the `cmd/gemini/` CLI (env-driven config, retrying HTTP via chassis `call`, structured logging, pretty-printed JSON output). Test suite is mock-based with 50 unit tests.
- **Not built here (by design):** streaming responses, multi-turn/chat history, multimodal inputs (images/audio), function-calling tool definitions beyond Google Search, and any provider-agnostic routing. These would be added only if a concrete consumer requires them; cross-provider concerns stay in `airborne`.
