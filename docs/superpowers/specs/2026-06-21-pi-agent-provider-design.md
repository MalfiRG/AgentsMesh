# Pi Agent Provider — Design Spec

**Date:** 2026-06-21
**Status:** Rev 2 (post adversarial review) — pending final spec review
**Goal:** Add the `pi` coding agent (gpt-5.x via the `openai-codex` provider) as
a selectable, builtin agent provider in AgentsMesh, behaving like Claude Code.

## Revision history

- **Rev 1:** Initial PTY-first design.
- **Rev 2:** Fixes from a 3-reviewer adversarial pass (2× Sonnet + Codex), which
  converged on these blockers, all now resolved below:
  1. **Agent HOME isolation was missing.** pi must use `RegisterAgentHome`
     (pi honors `PI_CODING_AGENT_DIR`, exactly like codex's `CODEX_HOME`), or
     credentials/sessions leak from the runner's shared `~/.pi` into every pod
     (cross-pod shared auth + wrong token attribution). Moved into v1 scope.
  2. **`PI_API_KEY` was fabricated.** pi has no such env var. Auth is
     `~/.pi/agent/auth.json` (`openai-codex` OAuth). Removed; replaced with the
     home-copy of `auth.json` + an optional real `OPENAI_API_KEY` (mirrors codex).
  3. **Default provider is `google`.** A bare `pi` launch defaults to Gemini.
     The agentfile now pins `--provider openai-codex` + a model.
  4. **Parser looked in the wrong place.** pi sessions live under the config
     dir (`PI_CODING_AGENT_DIR/sessions/<cwd-hash>/*.jsonl`), not workspace.
     Parser now reads the pod-local home dir.
  5. **`down.sql` would FK-fail.** Now clears `organization_agent_configs` /
     `organization_agents` / `user_agent_configs` first (cursor-cli template).
  6. **`parser_contract_test.go` coverage map** must gain a `"pi"` fixture
     entry or `bazel test //runner/...` fails. Added to scope.

## Summary

AgentsMesh supports pluggable agent providers (claude, codex, cursor, gemini,
aider, opencode, loopal). The frontend auto-discovers agents from the backend
API, so **no UI changes are required**. Claude Code runs `supported_modes =
"pty,acp"`; this spec delivers **PTY mode first** (the user's stated
precedence). ACP parity is deferred (Phase 2) — pi's `--mode rpc` is a custom,
undocumented protocol, not standard ACP.

The closest existing template is **codex** (custom-config agent needing HOME
isolation, optional API key, PTY+ACP), not the simpler aider/cursor agents.

## Scope

### In scope (v1, PTY)

1. Runner package `runner/internal/agents/pi/`:
   - `register.go` — `init()` registering process names, the token parser, and
     **`agentkit.RegisterAgentHome`** for `PI_CODING_AGENT_DIR` isolation.
   - `parser.go` + `testsupport/fixture.go` + `testdata/` — `piParser` reading
     token usage from pi's session JSONL in the **pod-local** home dir.
   - `BUILD.bazel` — Go library + test target.
2. Blank import in `runner/internal/runner/agents_import.go`.
3. **`runner/internal/tokenusage/parser_contract_test.go`** — add a `"pi"`
   entry to the `fixtureCases` map pointing at `pifixture.BuildFixtureSandbox`
   (the `TestRegistryCoverage_EveryNonOptOutParserHasFixture` guard fails the
   build otherwise).
4. Backend migration `000160_add_pi_agent.{up,down}.sql` — INSERT into `agents`
   with the provider/model/home agentfile; FK-safe DELETE on down.

### Out of scope (v1) — deferred follow-ups

- **ACP transport** (`transport.go`/`types.go`/`handler.go`). Phase 2,
  codex-style, after reverse-engineering pi `--mode rpc`.
- **MCP injection** (`MergeConfig` on the home spec). v1 copies pi's existing
  `~/.pi/agent` (which already carries the user's `mcp.json`) but does not
  inject AgentsMesh-managed MCP servers. `RegisterAgentHome` is registered with
  `MergeConfig: nil` in v1.
- **Input adapter.** Added only if pi's TUI emits bytes needing sanitizing
  (codex needed one; decided during implementation by observing pi PTY output).

## Design

### Provider identity & auth

- `slug = 'pi-cli'` (matches `claude-code`/`codex-cli`/`cursor-cli`/`gemini-cli`).
  Satisfies the `agents.slug` CHECK regex (migration 000139).
- Binary / `AGENT` / `EXECUTABLE` / `launch_command = 'pi'` (real binary; slug ≠
  binary, per cursor-cli 000157's ENOENT warning).
- `name = 'Pi'`, `supported_modes = 'pty'`, `is_builtin = true`, `is_active = true`.
- **Provider/model:** pinned `--provider openai-codex`; model chosen via a
  `CONFIG model SELECT(...)` (default a current `gpt-5.x`). Verified available:
  `pi --list-models` lists `openai-codex` → `gpt-5.5`, `gpt-5.4`,
  `gpt-5.3-codex-spark`, etc.
- **Auth:** `openai-codex` is OAuth (`~/.pi/agent/auth.json`, no API key). The
  runner host's `auth.json` is copied into each pod via `RegisterAgentHome`
  (same mechanism as codex copying `~/.codex`). `OPENAI_API_KEY` is declared as
  an optional secret for the key-based openai path (pi genuinely reads it; this
  mirrors codex's agentfile), but the primary path is the copied OAuth token.

### Runner package `runner/internal/agents/pi/`

`register.go` (codex pattern — parser + home isolation, no transport in v1):

```go
func init() {
    tokenusage.RegisterParser([]string{"pi", "pi-cli"}, &piParser{})
    agentkit.RegisterAgentHome(agentkit.AgentHomeSpec{
        EnvVar:      "PI_CODING_AGENT_DIR",
        UserDirName: ".pi/agent",
        MergeConfig: nil, // MCP injection deferred (Phase 2)
    })
    agentkit.RegisterProcessNames("pi")
}
```

Multi-slug `RegisterParser` (`"pi"` + `"pi-cli"`) is the **codex** pattern
(`codex`/`codex-cli`), future-proofing against collection keyed on either the
launch command or the slug. (Rev 1 mislabeled this the "aider pattern"; aider
registers a single slug.)

`parser.go` — `piParser` implements `tokenusage.TokenParser`:
`Parse(sandboxPath string, podStartedAt time.Time) (*tokenusage.TokenUsage, error)`
(signature verified at `runner/internal/tokenusage/parser.go:64`). It reads the
**pod-local** home set by the agentfile: `sandboxPath + "/pi-home/sessions/<cwd-hash>/*.jsonl"`
(pi defaults sessions under `PI_CODING_AGENT_DIR`), gated by
`tokenusage.IsModifiedAfter(path, podStartedAt)`. Returns `nil, nil` when empty.
pi session files are JSONL; usage is carried on message records
(`message.usage` with input/output/cache fields).

> **Implementation confirmation step (small):** before finalizing the parser,
> confirm the exact JSONL usage field names by capturing one pi session in the
> pod-local dir. The session location is already resolved (above); only the
> field mapping needs a fixture capture.

`testsupport/fixture.go` + `testdata/` — captured pi session JSONL fixture for a
deterministic parser contract test (claude/codex pattern).

`BUILD.bazel` — `go_library` (register.go, parser.go) + `go_test` + `testsupport`
sub-target, modeled on `runner/internal/agents/codex/BUILD.bazel`.

### Central registration

`runner/internal/runner/agents_import.go` — add blank import:
`_ "github.com/anthropics/agentsmesh/runner/internal/agents/pi"`.

### Backend migration `000160_add_pi_agent`

`up.sql` (codex agentfile shape, from migration 000132):

```sql
INSERT INTO agents (slug, name, launch_command, executable, is_builtin, is_active, supported_modes, agentfile_source)
VALUES ('pi-cli', 'Pi', 'pi', 'pi', true, true, 'pty',
  E'# === Identity ===\nAGENT pi\nEXECUTABLE pi\n\n# === Mode ===\nMODE pty\n\n# === Configuration ===\nCONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"\n\n# === Environment ===\nENV OPENAI_API_KEY SECRET OPTIONAL\nENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"\n\n# === Prompt ===\nPROMPT_POSITION prepend\n\n# === Build Logic ===\narg "--provider" "openai-codex"\narg "--model" config.model when config.model != ""\n');
```

> The exact agentfile-DSL `arg`/`CONFIG SELECT`/`ENV = sandbox.root + ...`
> syntax is copied verbatim from the live codex row (migration 000132); pi's
> only differences are the provider value and the home env var name.

`down.sql` (FK-safe, cursor-cli 000157 template):

```sql
BEGIN;
DELETE FROM organization_agent_configs WHERE agent_slug = 'pi-cli';
DELETE FROM organization_agents       WHERE agent_slug = 'pi-cli';
DELETE FROM user_agent_configs        WHERE agent_slug = 'pi-cli';
DELETE FROM agents                    WHERE slug = 'pi-cli';
COMMIT;
```

### Data flow (auto-discovery, unchanged)

1. Migration inserts the `pi-cli` builtin row.
2. Backend `agent_repo.ListBuiltinActive()` returns it via the agents API.
3. `clients/web` `listAgents()` → `AgentSelect.tsx` renders it, no code change.
4. Pod create with `pi-cli`: backend evaluates the agentfile → `LaunchCommand =
   pi --provider openai-codex --model <sel>` + pod env `PI_CODING_AGENT_DIR =
   <sandbox>/pi-home`; the runner copies the host's `~/.pi/agent` into
   `<sandbox>/pi-home` (auth + config), then exec()s pi in a PTY; `piParser`
   reads usage from `<sandbox>/pi-home/sessions/`.

## Testing

- **Runner unit:** `bazel test //runner/internal/agents/pi:pi_test` — parser
  totals match the JSONL fixture. `bazel test //runner/internal/tokenusage:...`
  must pass with the new `fixtureCases["pi"]` entry.
- **Migration:** `bazel run //deploy/dev:up` applies 000160; assert `pi-cli` row
  present; `down` removes it without FK error even when an org has enabled it.
- **End-to-end (manual, strengthened):** create a pi pod, **send one prompt and
  receive a model response** (not merely "TUI renders" — a bare TUI can render
  while auth/provider is broken), and confirm token usage is recorded.
  Prerequisite: the runner host has a valid `~/.pi/agent/auth.json`.

## Risks / Open Questions

1. **Dev-runner HOME isolation.** CLAUDE.md notes the dev runner runs with an
   isolated `$HOME` so its `~/.claude/*` writes don't touch real configs. For pi
   pods to authenticate in dev, that isolated HOME must contain a valid
   `~/.pi/agent/auth.json`. Documented as a dev-setup prerequisite, not a code
   change.
2. **pi rpc protocol (blocks ACP, not PTY).** Custom envelope
   (`{"type":"request"/"response",...}` + `extension_ui_request`), not JSON-RPC
   2.0 ACP. Phase 2 reverse-engineers it. PTY v1 does not depend on it.
3. **Session usage field shape** — small fixture-capture confirmation (above);
   location already resolved.

## Phase 2 — ACP (deferred, iterative)

Protocol-discovery spike mapping pi `--mode rpc` to the nine `acp.Transport`
methods + `EventCallbacks`; then `transport.go`/`types.go`/`handler.go` modeled
on codex; register via `acp.RegisterAgent("pi", "pi-rpc", factory)`; flip the
row to `supported_modes = 'pty,acp'` and add `MODE acp "pi-rpc"` to the
agentfile. If discovery shows pi's rpc mode cannot cover a required ACP
capability, stop at the documented gap — PTY v1 still stands.

## Reference patterns (file:line)

- Home isolation: `runner/internal/agentkit/home.go` (`AgentHomeSpec`),
  `runner/internal/agents/codex/register.go:24` (`RegisterAgentHome`)
- Agentfile with HOME env + args: `backend/migrations/000132_fix_codex_resume_agentfile.up.sql:9`
- PTY + parser register: `runner/internal/agents/aider/register.go`
- Parser interface: `runner/internal/tokenusage/parser.go:64`
- Parser fixture-coverage guard: `runner/internal/tokenusage/parser_contract_test.go`
- Central import list: `runner/internal/runner/agents_import.go`
- Slug CHECK constraint: `backend/migrations/000139_agents_slug_check.up.sql`
- FK-safe down migration: `backend/migrations/000157_add_cursor_cli_agent.down.sql`
- Frontend auto-discovery: `clients/web/src/components/pod/CreatePodForm/AgentSelect.tsx`,
  `clients/web/src/lib/api/connect/agentConnect.ts:119`
