# Pi Agent Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `pi` coding agent (gpt-5.x via the `openai-codex` provider) as a builtin, PTY-mode agent provider in AgentsMesh, behaving like Claude Code.

**Architecture:** A new runner package `runner/internal/agents/pi/` registers a token-usage parser, agent-HOME isolation (`PI_CODING_AGENT_DIR`, mirroring codex's `CODEX_HOME`), and process names. A backend migration inserts one builtin `agents` row whose agentfile pins `--provider openai-codex` + a model and points pi's config/session dir at a pod-local path. The frontend auto-discovers the agent from the API — no UI changes.

**Tech Stack:** Go (runner, backend), golang-migrate SQL migrations, Bazel build, the AgentsMesh agentfile DSL.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-06-21-pi-agent-provider-design.md` (Rev 2).
- Every Go file under 200 lines; test files under 400 lines (CLAUDE.md).
- No `what`-comments; only non-obvious `why` (CLAUDE.md).
- No `U+2014`/`U+2013` dashes in any output (ASCII hyphen only).
- Slug is `pi-cli`; binary / launch command / AGENT / EXECUTABLE is `pi`.
- Provider is `openai-codex`; default model `gpt-5.5`.
- Token parser must attribute usage to model `gpt-5.5` with input>0 and output>0 on its fixture (contract-test requirement).
- All `git` runs from the worktree root; use worktree-relative paths for file writes.
- Next free migration number is `000160` (latest is `000159`).

---

### Task 1: piParser — token usage from pi session JSONL

**Files:**
- Create: `runner/internal/agents/pi/parser.go`
- Create: `runner/internal/agents/pi/parser_test.go`
- Create: `runner/internal/agents/pi/testsupport/fixture.go`
- Create: `runner/internal/agents/pi/testsupport/testdata/pi_session.jsonl`
- Create: `runner/internal/agents/pi/BUILD.bazel`

**Interfaces:**
- Consumes: `tokenusage.TokenParser` interface — `Parse(sandboxPath string, podStartedAt time.Time) (*tokenusage.TokenUsage, error)` (`runner/internal/tokenusage/parser.go:64`); `tokenusage.NewTokenUsage()`, `(*TokenUsage).Add(model string, input, output, cacheCreation, cacheRead int64)`, `(*TokenUsage).IsEmpty()`, `tokenusage.IsModifiedAfter(path string, t time.Time) bool`.
- Produces: type `piParser struct{}` implementing `tokenusage.TokenParser`; `testsupport.BuildFixtureSandbox(t *testing.T) string` (plants the fixture under `<sandbox>/pi-home/sessions/<dir>/*.jsonl` and returns the sandbox path).

pi session facts (verified against a real `~/.pi/agent/sessions/.../*.jsonl`):
- JSONL; record discriminated by top-level `"type"`. Token usage lives only on `"type":"message"` records.
- A message record: `{"type":"message","message":{"model":"gpt-5.5","role":"assistant","usage":{"input":17341,"output":177,"cacheRead":25600,"cacheWrite":0,"totalTokens":...,"cost":{...}}}}`.
- User/zero turns carry `usage` all-zero; only assistant turns have positive counts. Each assistant message's `usage` is that API call's usage — summing across messages is correct (same model as codex/claude parsers).
- Mapping to `Add`: `input→input`, `output→output`, `cacheWrite→cacheCreation`, `cacheRead→cacheRead`.

- [ ] **Step 1: Write the fixture JSONL**

Create `runner/internal/agents/pi/testsupport/testdata/pi_session.jsonl` (exact bytes; the noise lines verify the parser ignores non-message and zero-usage records):

```jsonl
{"type":"session","id":"019ee1a7","timestamp":"2026-06-19T20:52:21.863Z"}
{"type":"model_change","provider":"openai-codex","modelId":"gpt-5.5"}
{"type":"message","message":{"role":"user","model":"","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0}}}
{"type":"message","message":{"role":"assistant","model":"gpt-5.5","usage":{"input":17341,"output":177,"cacheRead":25600,"cacheWrite":0}}}
{"type":"message","message":{"role":"assistant","model":"gpt-5.5","usage":{"input":2048,"output":512,"cacheRead":0,"cacheWrite":1024}}}
```

- [ ] **Step 2: Write the failing test**

Create `runner/internal/agents/pi/parser_test.go`:

```go
package pi

import (
	"testing"
	"time"

	"github.com/anthropics/agentsmesh/runner/internal/agents/pi/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPiParser_SumsAssistantUsageByModel(t *testing.T) {
	sandbox := testsupport.BuildFixtureSandbox(t)

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.False(t, usage.IsEmpty())

	m := usage.Models["gpt-5.5"]
	require.NotNil(t, m, "expected gpt-5.5 attribution, got %v", usage.Models)
	assert.Equal(t, int64(17341+2048), m.InputTokens)
	assert.Equal(t, int64(177+512), m.OutputTokens)
	assert.Equal(t, int64(25600), m.CacheReadTokens)
	assert.Equal(t, int64(1024), m.CacheCreationTokens)
}

func TestPiParser_NoSessions_ReturnsNil(t *testing.T) {
	usage, err := (&piParser{}).Parse(t.TempDir(), time.Unix(0, 0))
	require.NoError(t, err)
	assert.Nil(t, usage)
}
```

Create `runner/internal/agents/pi/testsupport/fixture.go`:

```go
// Package testsupport provides testing-only fixture helpers for the pi agent.
// Kept separate so the production pi library does not embed fixture bytes.
package testsupport

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

//go:embed testdata/pi_session.jsonl
var fixtureSession []byte

// BuildFixtureSandbox plants the pi session fixture in the pod-local layout the
// pi parser scans (`<sandbox>/pi-home/sessions/<cwd-hash>/`) and returns the
// sandbox path for parser.Parse(...).
func BuildFixtureSandbox(t *testing.T) string {
	t.Helper()
	sandbox := t.TempDir()
	dir := filepath.Join(sandbox, "pi-home", "sessions", "--fixture--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("pi fixture: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), fixtureSession, 0o644); err != nil {
		t.Fatalf("pi fixture: write: %v", err)
	}
	return sandbox
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd runner && go test ./internal/agents/pi/... -run TestPiParser -v`
Expected: FAIL — `undefined: piParser`.

- [ ] **Step 4: Implement the parser**

Create `runner/internal/agents/pi/parser.go`:

```go
package pi

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/agentsmesh/runner/internal/logger"
	"github.com/anthropics/agentsmesh/runner/internal/tokenusage"
)

type piParser struct{}

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

type piEntry struct {
	Type    string `json:"type"`
	Message struct {
		Model string  `json:"model"`
		Usage piUsage `json:"usage"`
	} `json:"message"`
}

// Parse sums per-message token usage from pi session JSONL files in the
// pod-local config dir (PI_CODING_AGENT_DIR, materialized at
// <sandbox>/pi-home by the agentfile + RegisterAgentHome).
func (p *piParser) Parse(sandboxPath string, podStartedAt time.Time) (*tokenusage.TokenUsage, error) {
	usage := tokenusage.NewTokenUsage()
	if sandboxPath == "" {
		return nil, nil
	}
	sessionsDir := filepath.Join(sandboxPath, "pi-home", "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil, nil
	}

	walkErr := filepath.WalkDir(sessionsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if !tokenusage.IsModifiedAfter(path, podStartedAt) {
			return nil
		}
		if perr := parsePiSessionFile(path, usage); perr != nil {
			logger.Pod().Warn("Pi parser: file parse error", "file", path, "error", perr)
		}
		return nil
	})
	if walkErr != nil {
		logger.Pod().Warn("Pi parser: walk error", "dir", sessionsDir, "error", walkErr)
	}

	if usage.IsEmpty() {
		return nil, nil
	}
	return usage, nil
}

func parsePiSessionFile(path string, usage *tokenusage.TokenUsage) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e piEntry
		if json.Unmarshal(line, &e) != nil || e.Type != "message" {
			continue
		}
		u := e.Message.Usage
		if e.Message.Model == "" || (u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheWrite == 0) {
			continue
		}
		usage.Add(e.Message.Model, u.Input, u.Output, u.CacheWrite, u.CacheRead)
	}
	return scanner.Err()
}
```

- [ ] **Step 5: Write the BUILD.bazel**

Create `runner/internal/agents/pi/BUILD.bazel` (modeled on `runner/internal/agents/codex/BUILD.bazel` — confirm the exact `go_library`/`go_test`/`embed` macro names and deps labels in that file and copy them, adjusting srcs/import path):

```python
load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "pi",
    srcs = [
        "parser.go",
        "register.go",
    ],
    importpath = "github.com/anthropics/agentsmesh/runner/internal/agents/pi",
    visibility = ["//visibility:public"],
    deps = [
        "//runner/internal/acp",  # remove if register.go does not import acp in v1
        "//runner/internal/agentkit",
        "//runner/internal/logger",
        "//runner/internal/tokenusage",
    ],
)

go_test(
    name = "pi_test",
    srcs = ["parser_test.go"],
    embed = [":pi"],
    deps = [
        "//runner/internal/agents/pi/testsupport",
        "@com_github_stretchr_testify//assert",
        "@com_github_stretchr_testify//require",
    ],
)
```

Create `runner/internal/agents/pi/testsupport/BUILD.bazel` (model on `runner/internal/agents/codex/testsupport/BUILD.bazel`):

```python
load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "testsupport",
    srcs = ["fixture.go"],
    embedsrcs = ["testdata/pi_session.jsonl"],
    importpath = "github.com/anthropics/agentsmesh/runner/internal/agents/pi/testsupport",
    visibility = ["//visibility:public"],
    deps = [],
)
```

> Note: `register.go` is created in Task 2 but listed in `srcs` now so the
> library target is stable. If running Task 1's test via `go test` (not Bazel)
> before Task 2, temporarily omit `register.go` from `srcs`, or create an empty
> `package pi` `register.go` stub first. Prefer running `bazel run //:gazelle`
> after Task 2 to regenerate BUILD files instead of hand-maintaining them.

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd runner && go test ./internal/agents/pi/... -run TestPiParser -v`
Expected: PASS (both tests).

- [ ] **Step 7: Commit**

```bash
git add runner/internal/agents/pi/parser.go runner/internal/agents/pi/parser_test.go \
        runner/internal/agents/pi/testsupport/ runner/internal/agents/pi/BUILD.bazel
git commit -m "feat(runner): pi token-usage parser"
```

---

### Task 2: Register pi (parser + HOME isolation + process names) and wire it in

**Files:**
- Create: `runner/internal/agents/pi/register.go`
- Modify: `runner/internal/runner/agents_import.go` (add blank import to the existing import block)
- Modify: `runner/internal/tokenusage/parser_contract_test.go` (add `"pi"` to `fixtureCases`; add `pifixture` import)

**Interfaces:**
- Consumes: `tokenusage.RegisterParser(slugs []string, p tokenusage.TokenParser)`; `agentkit.RegisterAgentHome(agentkit.AgentHomeSpec{EnvVar, UserDirName string, MergeConfig func(configPath, platformContent string) error})`; `agentkit.RegisterProcessNames(names ...string)`.
- Produces: package `init()` registering slugs `"pi"` + `"pi-cli"` to `&piParser{}`, the `PI_CODING_AGENT_DIR` home spec, and process name `"pi"`.

- [ ] **Step 1: Write register.go**

Create `runner/internal/agents/pi/register.go`:

```go
package pi

import (
	"github.com/anthropics/agentsmesh/runner/internal/agentkit"
	"github.com/anthropics/agentsmesh/runner/internal/tokenusage"
)

func init() {
	tokenusage.RegisterParser([]string{"pi", "pi-cli"}, &piParser{})

	// pi reads its config (incl. auth.json) and writes sessions under
	// PI_CODING_AGENT_DIR; isolate it per-pod so credentials and token
	// attribution are pod-scoped, not shared via the runner's real ~/.pi.
	agentkit.RegisterAgentHome(agentkit.AgentHomeSpec{
		EnvVar:      "PI_CODING_AGENT_DIR",
		UserDirName: ".pi/agent",
		MergeConfig: nil, // MCP injection deferred (Phase 2)
	})

	agentkit.RegisterProcessNames("pi")
}
```

- [ ] **Step 2: Add the blank import**

In `runner/internal/runner/agents_import.go`, add this line inside the existing blank-import block (keep alphabetical with the others):

```go
	_ "github.com/anthropics/agentsmesh/runner/internal/agents/pi"
```

- [ ] **Step 3: Add the contract-test fixture entry (failing first)**

In `runner/internal/tokenusage/parser_contract_test.go`, add the import alias next to the other fixture imports:

```go
	pifixture "github.com/anthropics/agentsmesh/runner/internal/agents/pi/testsupport"
```

and add this entry to the `fixtureCases` map literal:

```go
	"pi": {buildFixture: pifixture.BuildFixtureSandbox, wantModelNames: []string{"gpt-5.5"}},
```

- [ ] **Step 4: Run the contract test to verify coverage passes**

Run: `cd runner && go test ./internal/tokenusage/... -run 'TestRegistryCoverage_EveryNonOptOutParserHasFixture|TestRegisteredParsers_HaveFixtureProducingNonZeroTokens/pi' -v`
Expected: PASS — the `pi` subtest produces non-zero `gpt-5.5` input/output, and the coverage sentinel no longer flags `pi`.

> If this FAILS with "registered parser, no fixture", the blank import (Step 2)
> is missing or the fixture entry slug does not resolve to the same parser
> instance — verify `register.go` registers `"pi"` and the import path is exact.

- [ ] **Step 5: Build the runner to confirm registration links**

Run: `cd runner && go build ./...` (or `bazel run //:gazelle && bazel build //runner/...`)
Expected: builds clean; no `duplicate transport`/`duplicate agent home` panic at init (those would only fire at runtime, but the build must pass).

- [ ] **Step 6: Commit**

```bash
git add runner/internal/agents/pi/register.go runner/internal/runner/agents_import.go \
        runner/internal/tokenusage/parser_contract_test.go
git commit -m "feat(runner): register pi agent (parser, PI_CODING_AGENT_DIR home, process name)"
```

---

### Task 3: Backend migration — insert the `pi-cli` builtin agent row

**Files:**
- Create: `backend/migrations/000160_add_pi_agent.up.sql`
- Create: `backend/migrations/000160_add_pi_agent.down.sql`

**Interfaces:**
- Consumes: `agents` table columns `slug, name, launch_command, executable, is_builtin, is_active, supported_modes, agentfile_source` (`backend/internal/domain/agent/agent.go:17`); FK columns `organization_agents.agent_slug`, `organization_agent_configs.agent_slug` → `agents(slug)` (migration 000093); index column `user_agent_configs.agent_slug` (migration 000090).
- Produces: a builtin agent discoverable by `agent_repo.ListBuiltinActive()` and the frontend `listAgents()`.

- [ ] **Step 1: Write the up migration**

Create `backend/migrations/000160_add_pi_agent.up.sql` (agentfile DSL copied from the live codex row, migration 000132; provider/model/home adapted for pi):

```sql
-- 000160_add_pi_agent.up.sql
-- Register the pi coding agent (@earendil-works/pi-coding-agent) as a builtin.
--
-- slug='pi-cli' follows the claude-code / codex-cli naming convention, but
-- AGENT/EXECUTABLE/launch_command MUST be the real binary `pi` (the runner
-- exec()s LaunchCommand; using the slug would ENOENT every pod -- see 000157).
--
-- PTY-only first pass (ACP deferred). pi authenticates via openai-codex OAuth
-- in ~/.pi/agent/auth.json, copied per-pod into PI_CODING_AGENT_DIR by the
-- runner's RegisterAgentHome spec. --provider/--model are pinned because a bare
-- `pi` defaults to provider `google`. OPENAI_API_KEY is OPTIONAL schema metadata
-- (the agentfile evaluator skips `ENV X SECRET OPTIONAL` at eval time) for the
-- key-based openai path; the primary auth path is the copied OAuth token.
INSERT INTO agents (slug, name, launch_command, executable, is_builtin, is_active, supported_modes, agentfile_source)
VALUES ('pi-cli', 'Pi', 'pi', 'pi', true, true, 'pty',
  E'# === Identity ===\nAGENT pi\nEXECUTABLE pi\n\n# === Mode ===\nMODE pty\n\n# === Configuration ===\nCONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"\n\n# === Environment ===\nENV OPENAI_API_KEY SECRET OPTIONAL\nENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"\nENV PI_CODING_AGENT_SESSION_DIR = sandbox.root + "/pi-home/sessions"\n\n# === Prompt ===\nPROMPT_POSITION prepend\n\n# === Build Logic ===\narg "--provider" "openai-codex"\narg "--model" config.model when config.model != ""\n');
```

- [ ] **Step 2: Write the down migration (FK-safe)**

Create `backend/migrations/000160_add_pi_agent.down.sql` (cursor-cli 000157 template):

```sql
-- 000160_add_pi_agent.down.sql
-- Clear FK-referencing rows before removing the agents row, in a transaction.
-- organization_agents / organization_agent_configs hold NO ACTION FKs onto
-- agents(slug) (000093); user_agent_configs holds agent_slug without an FK
-- (000090) and is cleared to avoid orphan slug references.
BEGIN;

DELETE FROM organization_agent_configs WHERE agent_slug = 'pi-cli';
DELETE FROM organization_agents       WHERE agent_slug = 'pi-cli';
DELETE FROM user_agent_configs        WHERE agent_slug = 'pi-cli';
DELETE FROM agents                    WHERE slug = 'pi-cli';

COMMIT;
```

- [ ] **Step 3: Apply the migration and verify the row**

Run: `bazel run //deploy/dev:up` (auto-runs migrations), then:
`docker compose exec -T postgres psql -U <user> -d <db> -c "SELECT slug, name, launch_command, supported_modes FROM agents WHERE slug='pi-cli';"`
(Use the dev DB creds/port from the generated `.env`.)
Expected: one row — `pi-cli | Pi | pi | pty`.

- [ ] **Step 4: Verify the agentfile parses (no eval error)**

The backend evaluates `agentfile_source` when building a pod config. Confirm there is no agentfile parse/eval error in the backend log after the migration:
Run: `grep -i "agentfile\|pi-cli" deploy/dev/runtime/backend/backend.log | tail -20`
Expected: no parse/eval errors referencing `pi-cli`.

> If the agentfile fails to parse, check the DSL against the live codex row:
> `psql ... -c "SELECT agentfile_source FROM agents WHERE slug='codex-cli';"`
> and diff the directive syntax (`CONFIG ... SELECT(...)`, `arg ... when ...`,
> `ENV X = sandbox.root + "..."`).

- [ ] **Step 5: Verify down migration is FK-safe**

Run: `migrate -path backend/migrations -database "<dev-dsn>" down 1` then `up`.
Expected: down removes the row with no FK violation; up re-inserts it. (See CLAUDE.md "Database Migrations" for the exact `migrate` invocation; in dev the row is also restored by re-running `bazel run //deploy/dev:up`.)

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/000160_add_pi_agent.up.sql backend/migrations/000160_add_pi_agent.down.sql
git commit -m "feat(backend): add pi-cli builtin agent (PTY, openai-codex)"
```

---

### Task 4: End-to-end verification (manual gate)

**Files:** none (verification only).

**Interfaces:**
- Consumes: a running dev stack (`bazel run //deploy/dev:up`) and a runner host whose `~/.pi/agent/auth.json` holds a valid `openai-codex` OAuth token.

This task has no code; it is the real success gate the spec calls out (a bare TUI rendering is not proof the agent works).

- [ ] **Step 1: Confirm runner-host pi auth exists**

Run: `test -f ~/.pi/agent/auth.json && echo OK`
Expected: `OK`. (If the dev runner uses an isolated `$HOME` per CLAUDE.md, ensure that HOME contains a valid `~/.pi/agent/auth.json`, or pi pods cannot authenticate.)

- [ ] **Step 2: Create a pi pod from the web UI**

Run: `bazel run //clients/web:next_dev`, open the frontend, log in as `dev@agentsmesh.local / devpass123`, create a pod and select agent **Pi** from the picker.
Expected: "Pi" appears in the agent dropdown (auto-discovered); pod reaches running state.

- [ ] **Step 3: Send a prompt and confirm a response**

In the pod terminal, send a simple prompt (e.g. `say hello in one word`).
Expected: pi responds with a model-generated answer (proves provider/model/auth wiring, not just TUI render).

- [ ] **Step 4: Confirm token usage recorded**

After the turn, terminate the pod and check usage was collected:
Run: `grep -i "token\|usage\|pi-cli" deploy/dev/runtime/runner/runner.log | tail -20`
Expected: a non-empty token-usage record attributed to a `gpt-5.x` model for the pod. (Token files live in the pod-local `<sandbox>/pi-home/sessions/`; the parser runs on pod detach.)

- [ ] **Step 5: Confirm pod isolation (no leak into runner-host ~/.pi)**

Run: capture `ls ~/.pi/agent/sessions/` mtimes before/after the pod run.
Expected: the runner host's real `~/.pi/agent/sessions/` gains NO new session for the pod's cwd (the session landed in the pod-local `pi-home`), confirming HOME isolation works.

---

## Self-Review

**Spec coverage (Rev 2):**
- RegisterAgentHome / `PI_CODING_AGENT_DIR` — Task 2 Step 1.
- Real `openai-codex` OAuth auth + optional `OPENAI_API_KEY` (no fabricated `PI_API_KEY`) — Task 3 Step 1.
- Pinned `--provider openai-codex` + model — Task 3 Step 1.
- Pod-local session parser — Task 1 (parser reads `<sandbox>/pi-home/sessions`).
- FK-safe down migration — Task 3 Step 2.
- `parser_contract_test.go` fixture coverage — Task 2 Step 3.
- Strengthened E2E (send a prompt, confirm response + usage + isolation) — Task 4.
- Frontend zero-change auto-discovery — Task 4 Step 2 (verification only).
- ACP deferred — not in this plan (Phase 2).

**Placeholder scan:** No TBD/TODO; every code step has complete code. The two BUILD.bazel targets are concrete but flagged to regenerate via `bazel run //:gazelle` (the canonical mechanism in this repo) rather than hand-maintained — that is a real instruction, not a placeholder.

**Type consistency:** `piParser`, `piEntry`, `piUsage`, `parsePiSessionFile`, and `testsupport.BuildFixtureSandbox` are used identically across Tasks 1-2. `Add(model, input, output, cacheCreation, cacheRead)` argument order matches `token_usage.go`. Fixture model `gpt-5.5` matches the contract-test `wantModelNames`.
