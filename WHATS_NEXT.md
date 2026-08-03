# What's Next for ai_gemini_mod

`ai_gemini_mod` is a small, security-hardened Go library (`gemini/`) and CLI (`cmd/gemini/`) that wraps Google Gemini's `generateContent` REST endpoint, built on the shared `chassis-go` framework alongside its two siblings, `ai_claude_mod` (Anthropic) and `ai_openai_mod` (OpenAI), inside `ai_suite`. It is mature and well-tested (v1.3.14, 50+ unit tests) but its functional surface has barely moved since v1.0 — almost every version bump since has been a `chassis-go` dependency sync, not a capability addition — and it has fallen behind both its siblings and its own stated ecosystem role.

## Current State

- **Version 1.3.14**, Go 1.26.2, pinned to `chassis-go v11.1.8` via a local `replace github.com/ai8future/chassis-go/v11 => ../../chassis_suite/chassis-go`.
  - This module is not published or `go get`-able outside the monorepo, despite what its own README implies (see Risks).
- **Two components, one execution path.**
  - `gemini/client.go` (215 lines) + `gemini/types.go` (98 lines) form the library; `cmd/gemini/main.go` is a thin CLI wrapper.
  - Every request flows through exactly one path: `Generate` → `generateViaHTTP` → `doRequest`, POSTing JSON to `{baseURL}/{model}:generateContent`.
  - A shared `chassis-go-addons/llm` abstraction was adopted for standard requests in v1.3.6 and deliberately removed again in v1.3.13. `PRODUCT.md` now explicitly warns future changes not to reintroduce a second code path "without a deliberate decision."
- **Functional surface today:**
  - Single-shot prompt-in/text-out generation via `Generate(ctx, prompt, opts...)`.
  - Google Search grounding (`WithGoogleSearch`), on by default in the CLI via `GEMINI_GOOGLE_SEARCH=true`.
  - `maxOutputTokens` and `temperature` control, model selection, base URL override.
  - A pluggable `Doer` transport interface, used for retries (chassis `call.Client`), mocks, and instrumentation.
- **Security hardening is genuinely thorough:**
  - API key sent via the `x-goog-api-key` header — never the URL or a query string.
  - `baseURL` restricted to HTTPS; model names regex-validated (`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`) to block URL-path injection.
  - Response bodies capped at 10 MB; error bodies truncated to 1 KB in messages.
  - All errors classified through `chassiserrors.ValidationError` / `DependencyError` for consistent downstream handling.
- **Test coverage is solid for the surface that exists.**
  - `gemini/client_test.go` runs 732 lines of mock-based tests (no real API calls ever made).
  - `cmd/gemini/main_test.go` covers CLI config loading and defaults/overrides.
  - CHANGELOG reports 89.7% coverage on `gemini/` as of v1.3.11.
- **Confirmed real-world usage is minimal.** Grepping the whole `_code` tree turns up exactly one consumer:
  - `dispatch/internal/tools/llm.go` imports `ai_gemini_mod/gemini` via a local `replace ai_gemini_mod => ../ai_gemini_mod` in `dispatch/go.mod`.
  - It calls only `geminimod.New(apiKey, WithModel(model), WithTimeout(LLMClientTimeout))` and `client.Generate(ctx, fullPrompt, geminimod.WithMaxTokens(4096))` — no search grounding, no temperature override, no custom `Doer`.
  - That single call site is the entire real production usage pattern today.
- **The module's stated ecosystem role doesn't match reality.**
  - `PRODUCT.md` claims `ai_gemini_mod` "is the Gemini leg" of the `airborne` multi-provider gateway, and that without it "the gateway would need to embed raw HTTP logic for Gemini inline."
  - In fact, `airborne/go.mod` has no dependency on `ai_gemini_mod` at all. `airborne` ships its own independent `internal/provider/gemini` package and `internal/imagegen/gemini.go`, and has already had to fix a bug there (`airborne/_bugs_fixed/2026-07-04-gemini-filestore-bypassed-call-client.md`).
  - The "single hardened client for the suite" story is aspirational, not current fact.
- **Explicitly out of scope today, by design** (per `PRODUCT.md`'s "Not built here" list): streaming, multi-turn/chat history, multimodal input, function-calling tool definitions beyond Google Search, and provider-agnostic routing (the last of those correctly belongs in `airborne`).

### Suite capability comparison (as observed in each module's own README)

| Capability | ai_gemini_mod | ai_claude_mod | ai_openai_mod |
|---|---|---|---|
| Streaming | No | Yes (`StreamMessage`) | Yes (`ChatStream`, SSE) |
| Structured / JSON output | No | Yes | Via Responses API |
| File / vector store handling | No | Yes (`BetaFiles()`) | Yes (vector store CRUD + upload) |
| Multi-turn history in public API | No (wire format supports it, API doesn't expose it) | Yes | Yes |
| Custom function/tool calling | No (only built-in Google Search tool) | Yes (stable tools) | Yes (tool use) |
| Provider-native differentiator | Google Search grounding | Prompt caching, thinking controls | Images (DALL-E) |

This table is the clearest evidence that `ai_gemini_mod` is the least-developed of the three provider modules, not because Gemini's API is thinner, but because feature work here has consistently lost out to `chassis-go` version-tracking chores.

## What's Next

### Near-term

1. **Stop silently dropping Google Search grounding metadata — the module's own default-on feature.**
   - `GEMINI_GOOGLE_SEARCH` defaults to `true` in the CLI, but `gemini.Candidate` in `types.go` has no `GroundingMetadata` field.
   - Gemini's real `generateContent` response includes `groundingMetadata` (web search queries, grounding chunks/citations) on every grounded call; `doRequest`'s `json.Unmarshal(body, respBody)` silently discards any field the typed struct doesn't declare.
   - Because `doRequest` never returns the raw bytes to the caller — only the typed `*Response` — this data is unrecoverable anywhere downstream, including in the CLI's own `json.MarshalIndent(resp, ...)` output.
   - Net effect: users get grounded answers with no way to see or cite the sources that grounded them.
2. **Add `thoughtsTokenCount` and `cachedContentTokenCount` to `UsageMetadata`.**
   - The module's own default model, `gemini-3-pro-preview`, is a reasoning model that returns thinking-token counts.
   - Sibling repo `pricing_db` already ships dedicated Gemini-specific cost logic (`GeminiUsageMetadata`, `ThinkingCost`, a documented "cache_precedence" rule) that expects exactly this data.
   - `ai_gemini_mod`'s `UsageMetadata` only models `PromptTokenCount` / `CandidatesTokenCount` / `TotalTokenCount` — there is no way for a Go caller to get thinking-token or cached-token counts out of this library at all, with no raw-bytes fallback.
3. **Expose multi-turn conversation history.**
   - The wire format already supports it: `Request.Contents` is `[]Content`, and each `Content` already carries a `Role`.
   - `Generate(ctx, prompt string, ...)` always builds a single-item slice, so the capability is blocked purely at the public API layer, not the transport layer.
   - A `GenerateWithHistory(ctx, []Content, opts...)` method (or a `WithHistory(...Content)` `GenerateOption`) is a small, additive change to `client.go`.
4. **Add `systemInstruction` support.**
   - Gemini's API accepts a dedicated top-level `systemInstruction` field, separate from `contents`; it doesn't exist in `Request` today.
   - One struct field plus one `GenerateOption` (`WithSystemInstruction`) — cheap to add, and one of the most commonly needed controls for any agent-facing use case.
5. **Add the missing common `GenerationConfig` fields:** `topP`, `topK`, `stopSequences`, and especially `responseMimeType`/`responseSchema` for JSON/structured output.
   - Both `ai_claude_mod` and `ai_openai_mod` already advertise structured output as a supported surface (see comparison table above); `ai_gemini_mod` has none.
   - `dispatch`, a tool-orchestration runtime, is exactly the kind of consumer that benefits from reliable JSON-mode output instead of parsing free text out of `resp.Text()`.
6. **Fix the README's installation instructions.**
   - `README.md` tells readers to run `go get ai_gemini_mod/gemini`, but the module isn't published — it's only ever consumed via a local `replace` directive inside this monorepo, same as every sibling.
   - `ai_claude_mod/README.md` already documents this correctly ("`go get ai_claude_mod` is not a valid external installation command"); `ai_gemini_mod`'s own README still tells a new engineer to do the wrong thing.
7. **Add a `CountTokens` client method** against Gemini's `:countTokens` endpoint, using the same `Doer`/validation plumbing `Generate` already has.
   - `pricing_db` documents Gemini's 200K-token pricing-tier break as its single highest-risk-of-miscalculation case across every provider it supports.
   - Today, nothing using this module can check token count before spending, so no caller can pre-flight-check which pricing tier a request will land in.

### Mid-term

1. **Streaming (`streamGenerateContent` / SSE).**
   - Both sibling modules already have it: `ai_claude_mod` exposes `StreamMessage`, `ai_openai_mod` has `ChatStream` with full SSE parsing.
   - `ai_gemini_mod` is the one provider client in the suite without it (see comparison table).
   - Because `PRODUCT.md` explicitly flags the single-execution-path design as intentional — a direct reaction to the v1.3.6/v1.3.13 abstraction-then-removal churn — this needs a considered decision about how a second, streaming code path coexists with that invariant, not an ad hoc bolt-on.
2. **Custom function-calling / tool declarations.**
   - `Tool` in `types.go` only models the built-in `GoogleSearch` tool; there is no `functionDeclarations` support.
   - `dispatch` is literally a tool-orchestration runtime — the most natural consumer for real function-calling — and today gets nothing here, so any Gemini-side tool-calling `dispatch` wants has to be hand-rolled outside this module.
3. **Multimodal input (`inlineData` / `fileData` parts).**
   - `Part` currently has exactly one field: `Text string`.
   - `airborne`'s independent native Gemini provider already handles image generation and FileSearchStore-backed RAG outside this module entirely.
   - Folding that support back into `ai_gemini_mod` is a concrete, achievable step toward the "single hardened client" mission `PRODUCT.md` claims but doesn't currently deliver.
4. **A Gemini File API / FileSearchStore client surface**, mirroring what `ai_openai_mod` already does for OpenAI (vector-store CRUD plus two-step multipart file upload).
   - `airborne` is presently maintaining equivalent logic natively and independently, and already had to fix a bug there (`airborne/_bugs_fixed/2026-07-04-gemini-filestore-bypassed-call-client.md`) — a concrete signal this is being built and maintained twice in the same organization.
5. **`safetySettings` override support**, so per-tenant or per-consumer content-safety thresholds can be set through this client rather than relying only on Gemini's server-side defaults.
   - `airborne`'s per-tenant configuration model already wants this kind of control for its other providers.

### Long-term

1. **Make this the Gemini integration `airborne` actually uses.**
   - Once streaming, function calling, multimodal input, and file-store support close the gap with `airborne`'s native provider, migrate `airborne/internal/provider/gemini` and `airborne/internal/imagegen/gemini.go` onto `ai_gemini_mod`.
   - This retires a second, independently-maintained Gemini HTTP client and finally delivers the "reduce integration risk" goal `PRODUCT.md` already states as this module's reason for existing.
2. **`thinkingConfig` support** (thinking budget, include-thoughts toggle).
   - Lets callers control the cost/latency tradeoff of Gemini's reasoning behavior proactively, rather than only observing thinking-token spend after the fact once near-term item 2 is in place.
3. **Batch API support**, matching the batch-mode cost rules `pricing_db` already models for Gemini specifically.
   - Includes the documented incompatibility between batch mode and search grounding, useful for high-volume, latency-insensitive callers elsewhere in the suite.
4. **Context caching (`cachedContent`)** for repeated large-prefix prompts.
   - `pricing_db`'s Gemini-specific "cache_precedence" rule already prices this; the client has no way to create or reference a cached-content resource today.
5. **Decide this module's real distribution story.**
   - The local-`replace`-only pattern (shared by every sibling module) works fine while the only consumer is `dispatch` inside the same monorepo, but there is no actual "install this outside `_code`" path today despite what the README implies.
   - If any future consumer sits outside this workspace, module-path and versioning decisions need to be made deliberately, not discovered by a broken build.

## Product Opportunities

- **Become the real, single source of truth for Gemini in `ai_suite`.**
  - The biggest concrete opportunity is closing the gap between this module and `airborne`'s independent native implementation.
  - Every capability `airborne` had to build itself (file stores, image generation) because this module didn't have it is duplicated engineering effort and duplicated future security/API-change risk.
- **Be the accuracy backbone for Gemini cost tracking.**
  - `pricing_db` has unusually deep, Gemini-specific cost logic (tiered pricing at 200K tokens, thinking-token costs, grounding query costs, cache-precedence rules) sitting ready to consume data this module isn't currently capturing.
  - Closing the grounding-metadata and usage-metadata gaps (near-term items 1–2) turns this module into the clean, structured input `pricing_db` already expects, instead of requiring callers to somehow retain raw bytes this module never gives them.
- **Serve `dispatch`'s actual shape of need.**
  - The one confirmed consumer is a tool-orchestration runtime that today only does bare single-shot generation through this module.
  - Structured output and function-calling support (near-term item 5, mid-term item 2) are the two additions most directly aligned with what `dispatch` already is, not speculative "might be nice" features.
- **CLI as a fast internal eval/scripting tool.**
  - The CLI's always-emit-full-JSON behavior is a deliberate, documented choice (debuggability over end-user ergonomics), and that's fine as a default.
  - An opt-in `--text` flag (print `resp.Text()` only) or a `--stream` flag would make it far more useful for shell pipelines and quick model comparisons without weakening that default.
- **Reference implementation for future provider modules.**
  - Of the three provider clients in `ai_suite`, this one is the smallest and most narrowly scoped (`ai_claude_mod` has a modern/legacy/beta three-surface split; `ai_openai_mod` bundles chat, images, and vector stores).
  - If the suite adds a fourth provider, `ai_gemini_mod`'s current single-path simplicity is a reasonable template — provided the near/mid-term gaps above are closed first so it's a template worth copying rather than a template that propagates the same gaps.

## Risks / Blockers

- **Silent data loss on the module's own default configuration is the top risk.**
  - Grounding is on by default; a reasoning model is the default model; neither the grounding metadata nor the thinking/cached-token counts Gemini already returns for those defaults are captured anywhere in this module's types.
  - There is no raw-bytes escape hatch: `doRequest` unmarshals directly into the caller-supplied struct and never returns the original body to anyone.
  - Every future Gemini response field addition requires a coordinated `ai_gemini_mod` release before any consumer — including `pricing_db`-style cost tracking — can see it; nothing can work around this from outside the module.
- **`airborne` and `ai_gemini_mod` are two independently-maintained Gemini HTTP integrations today.**
  - Whichever one falls behind on a Gemini API change, a safety-relevant update, or a security fix rots quietly while the other moves on.
  - `PRODUCT.md`'s narrative that this module *is* the gateway's Gemini leg is not true today, which means anyone reading only `PRODUCT.md` will misjudge blast radius when planning changes here.
- **Reintroducing streaming risks repeating the v1.3.6 → v1.3.13 churn** — adding a shared abstraction, then ripping it back out one point release later — if it's done as a quick patch rather than a deliberate design decision. `PRODUCT.md` already flags this history explicitly as a caution for future maintainers.
- **Most of this repo's version history is dependency-sync chores, not product work.**
  - Of the ~24 CHANGELOG entries, the large majority are `chassis-go` major-version bumps (v4 through v11) with matching `RequireMajor` / `VERSION.chassis` updates.
  - Only a handful (v1.0.0, v1.2.0's security hardening pass, the v1.3.6/v1.3.13 addon experiment) are feature- or safety-shaped changes.
  - If that ratio continues, the "not built here" list (streaming, multi-turn, multimodal, tools) risks never getting addressed simply because contributor time keeps going to treading water on the shared framework.
- **The README actively misleads new contributors on installation** (`go get ai_gemini_mod/gemini` doesn't work outside the monorepo).
  - Small individually, but it compounds with the `PRODUCT.md`-vs-`airborne` mismatch above into a broader pattern: this repo's documentation describes an idealized state rather than the current one in a couple of specific, checkable places.
- **Distribution is monorepo-only by construction.**
  - The local `replace` directive to `../../chassis_suite/chassis-go`, combined with an unpublished module path, means this module cannot be adopted by any team or service outside `_code` without deliberate re-plumbing.
  - Acceptable today with a single internal consumer, but worth remembering before anyone promises this library externally.
- **Minor build-matrix gap:** `make build-all` only produces `linux-amd64` and `darwin-arm64` binaries even though `scripts/launcher.sh` is written to dispatch generically by OS/architecture — there's no `darwin-amd64` or Windows target, so the launcher's generic dispatch logic is untested for architectures the Makefile never builds.
