# Changelog

## [1.3.12] - 2026-03-28
- Add appversion.go with embedded VERSION for chassis SetAppVersion pattern
- Update cmd/gemini/main.go to call chassis.SetAppVersion(aigeminimod.AppVersion) before RequireMajor
- Remove dead `var version = "dev"` variable
- Agent: Claude Code (Claude:Opus 4.6)

## [1.3.11] - 2026-03-27
- Add 14 new unit tests covering addonBaseURL, boundary values, default config, short error passthrough, extended model name validation, and addon client branching logic
- Test count: 34 -> 48, coverage: 88.7% -> 89.7% for gemini/ package
- Agent: Claude:Opus 4.6

## [1.3.10] - 2026-03-26
- GO-BEST-PRACTICES conformance: Makefile with cross-platform build targets (build-linux, build-darwin, build-all), launcher script, binary naming, LDFLAGS with version injection, CGO_ENABLED=0 static builds
- Agent: Claude:Opus 4.6

## 1.3.9

- Replace raw errors (errors.New, fmt.Errorf) with chassis error factories in gemini/client.go
  - Input validation errors (API key, model, base URL, temperature, maxTokens) use chassiserrors.ValidationError
  - API/HTTP failures (do request, read response, HTTP 4xx/5xx) use chassiserrors.DependencyError with WithCause
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.8

- Upgrade chassis-go from v9 to v10 (10.0.2)
- Update RequireMajor(9) to RequireMajor(10) in main and tests
- Update all import paths from github.com/ai8future/chassis-go/v9 to /v10
- Update VERSION.chassis to 10.0.0
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.7

- Add missing chassis-go-addons/llm dependency to README dependencies table
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.6

- Replace hand-rolled Gemini HTTP calls with chassis-go-addons/llm module for standard (non-GoogleSearch) requests
- Keep doRequest fallback for GoogleSearch grounding and custom Doer injection (test mocks)
- Add chassis-go-addons/llm dependency
- All existing tests continue to pass (mock-based tests use the fallback path)
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.5

- Fix stale VERSION.chassis (was 8.0.0, now 9.0.0)
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.4

- Upgrade chassis-go from v8 to v9 (9.0.0)
- Update RequireMajor(8) to RequireMajor(9) in main and tests
- Add deploy package integration for environment detection and env file loading
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.3

- Upgrade chassis-go from v7 to v8 (8.0.0)
- Update RequireMajor(7) to RequireMajor(8) in main and tests
- Update all import paths to github.com/ai8future/chassis-go/v8
- Agent: Claude Code (Claude:Opus 4.6)

## 1.3.2

- Upgrade chassis-go from v6.0.9 to v7.0.0 (major version bump)
- Update RequireMajor gate from 6 to 7
- Update all import paths to github.com/ai8future/chassis-go/v7
- Add CLI registry pattern (registry.InitCLI / registry.ShutdownCLI)
- Update VERSION.chassis to 7.0.0
- Agent: Claude:Opus 4.6

## [1.3.1] - 2026-03-07
- Sync uncommitted changes

## 1.3.0

- Upgrade chassis-go from v5.0.0 to v6.0.9 (major version bump)
- Update RequireMajor gate from 5 to 6
- Update all import paths to github.com/ai8future/chassis-go/v6
- Update VERSION.chassis to 6.0.9
- Agent: Claude:Opus 4.6

## 1.2.1

- **Docs**: Add comprehensive README.md with library/CLI usage, API reference, security notes, and project structure
- Agent: Claude:Opus 4.6

## 1.2.0

- **Security**: Remove leaked API key from .env, replace with placeholder
- **Security**: Validate model name against regex to prevent URL path manipulation
- **Fix**: Reject empty model names in `New()` (previously produced malformed URLs)
- **Fix**: Add `role: "user"` to request Content per Gemini API spec
- **Fix**: Add upper-bound validation for temperature (max 2.0) and maxTokens (max 1,000,000)
- **Fix**: Sync go.sum with chassis-go/v5 dependencies (broken build)
- **Fix**: Derive context timeout from retry constants instead of hardcoded multiplier
- **Refactor**: Extract `run()` from `main()` for testability
- **Tests**: Add tests for model name validation, upper bounds, and role field
- Agent: Claude:Opus 4.6

## 1.1.0

- Upgrade chassis-go from v4.0.0 to v5.0.0 (module path migrated to /v5)
- Update RequireMajor gate from 4 to 5
- Update all import paths to github.com/ai8future/chassis-go/v5

## 1.0.1

- Upgrade chassis-go to v4.0.0 (module v1.4.0)

## 1.0.0

- Initial release: reusable Gemini API client library (`gemini/`) with Doer interface, functional options, and response types
- CLI wrapper (`cmd/gemini/`) using chassis-go for config, logging, and resilient HTTP calls
- Full test coverage for library (mock-based) and CLI config loading
