# Pi Agent Provider — Design Spec

**Date:** 2026-06-21
**Status:** Approved (brainstorming) — pending spec review
**Goal:** Add the `pi` coding agent (gpt-5.x via the openai-codex provider) as a
selectable, builtin agent provider in AgentsMesh, behaving like Claude Code.

## Summary

AgentsMesh already supports pluggable agent providers (claude, codex, cursor,
gemini, aider, opencode, loopal). Adding pi means slotting a new provider into
that same pattern. The frontend auto-discovers agents from the backend API, so
**no UI changes are required** — a new builtin row plus a runner package is
enough for pi to appear in the pod-creation picker.

Claude Code runs `supported_modes = "pty,acp"`. This spec delivers **PTY mode
first** (the precedence the user set); ACP parity is a deferred, iterative
follow-up tracked in "Phase 2 — ACP (deferred)" below.

## Scope

### In scope (v1, PTY)

1. Runner package `runner/internal/agents/pi/`:
   - `register.go` — `init()` registering process names and a token parser.
   - `parser.go` + `testsupport/fixture.go` + `testdata/` — `piParser` reading
     token usage from pi's session JSON.
   - `BUILD.bazel` — Go library + test target (modeled on aider's).
2. Blank import in `runner/internal/runner/agents_import.go`.
3. Backend migration `000160_add_pi_agent.{up,down}.sql` — one INSERT into
   `agents` (and a DELETE on down).

### Out of scope (v1) — deferred follow-ups

- **ACP transport** (`transport.go`/`types.go`/`handler.go`). pi's `--mode rpc`
  is a custom line protocol, not standard ACP — see "Phase 2" + "Risks".
- **MCP injection** (`home.go`). pi uses its own `~/.pi` config; we do not
  inject MCP in v1 (mirrors cursor-cli migration 000157's deliberate choice).
- **Subscription-based pod auth.** See "Risks / Open Questions".
- **Input adapter.** Added only if pi's TUI emits bytes that need sanitizing
  (codex needed one; aider/gemini did not). Decided during implementation.

## Design

### Provider identity

- `slug = 'pi-cli'` (matches the `claude-code` / `codex-cli` / `cursor-cli` /
  `gemini-cli` convention).
- Binary / `AGENT` / `EXECUTABLE` / `launch_command = 'pi'` (the real binary;
  slug ≠ binary, exactly as cursor-cli migration 000157 documents — using the
  slug as the launch command would ENOENT every pod).
- `name = 'Pi'`, `supported_modes = 'pty'`, `is_builtin = true`,
  `is_active = true`.

### Runner package `runner/internal/agents/pi/`

`register.go` (aider pattern — PTY + parser, no transport):

```go
func init() {
    tokenusage.RegisterParser([]string{"pi", "pi-cli"}, &piParser{})
    agentkit.RegisterProcessNames("pi")
}
```

`parser.go` — `piParser` implements `tokenusage.TokenParser`:
`Parse(sandboxPath string, podStartedAt time.Time) (*tokenusage.TokenUsage, error)`.
Reads pi's session storage (default `~/.pi/agent` sessions, or the pod's
`--session-dir`), gated by `tokenusage.IsModifiedAfter(path, podStartedAt)` so
only this pod's session is counted. Returns `nil, nil` when usage is empty.

> **Implementation discovery step (small):** confirm pi's session-file JSON
> shape and the field(s) carrying input/output/cache token counts before
> finalizing the parser. If pi's session format does not expose usage cleanly,
> fall back to `tokenusage.RegisterParserOptOut([]string{"pi"})` and add the
> parser in a follow-up — pi still runs, just without usage accounting. (The
> user chose to include the parser; this fallback is the contingency only.)

`testsupport/fixture.go` + `testdata/` — a captured pi session fixture so the
parser has a deterministic contract test (claude/codex/aider pattern).

`BUILD.bazel` — `go_library` (srcs: register.go, parser.go) + `go_test`
(parser_test.go) + the `testsupport` sub-target, modeled on
`runner/internal/agents/aider/BUILD.bazel`.

### Central registration

Add to `runner/internal/runner/agents_import.go` (blank-import list):

```go
_ "github.com/anthropics/agentsmesh/runner/internal/agents/pi"
```

This triggers the package `init()`.

### Backend migration `000160_add_pi_agent`

`up.sql` (cursor-cli row shape):

```sql
INSERT INTO agents (slug, name, launch_command, executable, is_builtin, is_active, supported_modes, agentfile_source)
VALUES ('pi-cli', 'Pi', 'pi', 'pi', true, true, 'pty',
  E'# === Identity ===\nAGENT pi\nEXECUTABLE pi\n\n# === Mode ===\nMODE pty\n\n# === Environment ===\nENV PI_API_KEY SECRET OPTIONAL\n\n# === Prompt ===\nPROMPT_POSITION prepend\n');
```

`down.sql`:

```sql
DELETE FROM agents WHERE slug = 'pi-cli';
```

The `ENV PI_API_KEY SECRET OPTIONAL` declaration is schema metadata only (the
agentfile evaluator skips `ENV X SECRET OPTIONAL` at eval time); it renders a
curated optional credential field in the frontend, mirroring gemini-cli's
`GOOGLE_API_KEY` and cursor-cli's `CURSOR_API_KEY`. The exact env var name is
confirmed against pi's headless-auth flag during implementation.

### Data flow (unchanged, auto-discovery)

1. Migration inserts the `pi-cli` builtin row.
2. Backend `agent_repo.ListBuiltinActive()` returns it via the agents API.
3. `clients/web` `listAgents()` → `AgentSelect.tsx` renders it with no code
   change.
4. User creates a pod with agent `pi-cli`; backend ships `launch_command=pi`
   to the runner; the runner exec()s `pi` in a PTY pod; `RegisterProcessNames`
   lets pod accounting recognize the process; `piParser` reports token usage.

## Testing

- **Runner:** `bazel test //runner/internal/agents/pi:pi_test` — parser unit
  test against the captured session fixture (token totals match expected).
  Cross-agent parser contract test if one exists (claude/codex have one).
- **Migration:** `bazel run //deploy/dev:up` applies 000160; assert the
  `pi-cli` row exists and `down` removes it (sequence/embed tests already guard
  numbering).
- **End-to-end (manual):** `bazel run //clients/web:next_dev`, create a pod,
  confirm "Pi" appears in the picker and a pod launches an interactive pi TUI.

## Risks / Open Questions

1. **pi rpc protocol is custom + undocumented (blocks ACP, not PTY).** Probing
   `pi --mode rpc` shows a bespoke envelope (`{"type":"request"/"response",...}`
   plus `extension_ui_request` widget/status events), not JSON-RPC 2.0 ACP.
   Phase 2 must reverse-engineer it and write a codex-style custom transport.
   PTY v1 does not depend on this.
2. **Pod authentication.** pi authenticates via `~/.pi` config / the
   openai-codex subscription. A pod runs with an isolated HOME, so pi's
   credentials must be provisioned into the pod (env bundle / mounted config)
   for pi to actually run. v1 declares an optional `PI_API_KEY` headless path;
   wiring subscription auth into pods is a separate follow-up.
3. **Session token-usage field shape** — see parser discovery step above.

## Phase 2 — ACP (deferred, iterative)

When pursued: protocol-discovery spike mapping pi `--mode rpc` to the nine
`acp.Transport` methods + `EventCallbacks`; then `transport.go` / `types.go` /
`handler.go` modeled on codex (custom JSON-over-stdio); register the transport
via `acp.RegisterAgent("pi", "pi-rpc", factory)`; flip the migration/row to
`supported_modes = 'pty,acp'`. If discovery shows pi's rpc mode cannot cover a
required ACP capability, stop at the documented gap — PTY v1 still stands.

## Reference patterns (file:line)

- Minimal PTY register: `runner/internal/agents/gemini/register.go`
- PTY + parser register: `runner/internal/agents/aider/register.go`
- Parser interface + impl: `runner/internal/agents/aider/parser.go`
- Central import list: `runner/internal/runner/agents_import.go`
- Migration template (PTY-only agent): `backend/migrations/000157_add_cursor_cli_agent.up.sql`
- ACP registration (Phase 2): `runner/internal/agents/codex/register.go`,
  `runner/internal/acp/transport.go` (`RegisterAgent`)
