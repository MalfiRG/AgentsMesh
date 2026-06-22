# Session 1 Handoff — AgentsMesh pi-agent provider

**Status at session open:** Branch `feat/pi-agent-provider` at `c4017c5b1` (pre-handoff; this doc's auto-commit sits one above). Spec Rev 2 + Plan Rev 2 committed and adversarially reviewed (verdict: shippable). **Zero implementation done** — only `docs/superpowers/specs/...` and `docs/superpowers/plans/...`.
**Primary path for Session 2:** Execute the implementation plan via **subagent-driven-development**, one task at a time, Task 1 → Task 5, with a review gate between tasks.
**Blocker class:** Nothing blocked. All 5 tasks have verified, placeholder-free code. The only live prerequisite is Task 4 (Docker runner) needing a fresh `openai-codex` OAuth token.

---

## Bootstrap prompt (paste into a fresh session)

```
Start subagent-driven implementation of the AgentsMesh pi-agent provider from Task 1.

**Worktree:** /home/malfirg/programming_projects/AgentsMesh/.claude/worktrees/declarative-swinging-puppy
  (branch `feat/pi-agent-provider`, at `c4017c5b1` + the handoff auto-commit on top)
**Status:** Spec + plan are done and reviewed (shippable). No code written yet. Adding `pi` (gpt-5.x via openai-codex) as a builtin PTY agent provider, modeled on the codex integration.

**Load-bearing docs (read in order):**
1. docs/superpowers/plans/2026-06-21-pi-agent-provider.md  (the plan — 5 tasks, exact code per step)
2. docs/superpowers/specs/2026-06-21-pi-agent-provider-design.md  (the why, Rev 2 revision history)

**Recovery if context is thin after compaction:**
`mempalace_search("AgentsMesh pi agent provider")` — palace is mined at compaction.

**Bootstrap (sanity checks with expected output):**
1. `git -C <worktree> log --oneline -2` (expect the handoff commit atop `c4017c5b1`; verify two-commit window, not a bare HEAD==SHA check)
2. `git -C <worktree> branch --show-current` (expect `feat/pi-agent-provider`)
3. `ls runner/internal/agents/pi/ 2>/dev/null` (expect: NO such dir yet — Task 1 creates it)
4. `ls backend/migrations/ | grep 000160` (expect: empty — `000160` is free, Task 3 creates it)
5. `test -f ~/.pi/agent/auth.json && echo OK` (expect `OK` — needed for Task 4/5 auth seeding)

**Execution method:** superpowers:subagent-driven-development — fresh subagent per task, two-stage review (impl subagent, then a separate review subagent) between tasks. Tasks are independently testable; do them in order (2 depends on 1; 5 depends on 1-4).

**Per-task gate:** each task ends with a real test/verify command in the plan. Do NOT mark a task done until its command passes with the expected output quoted in the plan.

**Safety constraints:**
- Use WORKTREE-RELATIVE paths for Write/Edit. Absolute paths missing the `.claude/worktrees/declarative-swinging-puppy/` segment silently land in the MAIN repo (this bit twice in Session 1). Verify with `git status --short` before each commit.
- BUILD.bazel files are gazelle-generated — never hand-write deps; run `bazel run //:gazelle` after editing Go imports (plan Task 1 Step 4, Task 2 Step 3).
- Commit per task with the message in each task's final step. Stay on `feat/pi-agent-provider`; do NOT push or merge to main without operator say-so.
- Task 4 mutates the dev Docker stack (rebuild runner image, seed auth). Confirm the openai-codex OAuth token is fresh BEFORE seeding (pi login needs a browser, impossible inside the container).

**Model tiering:** Haiku=verify, Sonnet=impl, Opus=arbitration.

After bootstrap, begin Task 1. Wait for operator confirmation only if a task's verify step fails unexpectedly.
```

---

## Settled (do NOT re-discuss)

### Closed in Session 1 (prevents re-litigation):
- **pi identity** = `@earendil-works/pi-coding-agent`, binary `pi`, slug `pi-cli`, provider `openai-codex`, default model `gpt-5.5`. Auth = OAuth in `~/.pi/agent/auth.json` (NOT an API key; `PI_API_KEY` does not exist — was a fabrication caught in spec review).
- **Integration template = codex** (not aider/cursor): needs `RegisterAgentHome` (`PI_CODING_AGENT_DIR`, analog of `CODEX_HOME`), pinned `--provider`/`--model` args, optional `OPENAI_API_KEY` env.
- **HOME-copy path contract (Codex-verified twice):** `RegisterAgentHome{EnvVar:"PI_CODING_AGENT_DIR", UserDirName:".pi/agent"}` + agentfile `ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"` → `copyDirSelective` lands files at `<sandbox>/pi-home/...` (NOT a `/agent` subdir) and SKIPS top-level `sessions/`+`cache/`. Pod-written sessions land at `<sandbox>/pi-home/sessions/<cwd-hash>/*.jsonl` — exactly where the parser walks. Sandbox survives until token collection. This path is correct; do not redesign it.
- **Dev runner runs in Docker** (CLAUDE.md "host-side mode" is STALE for the runner). pi must be npm-installed in `deploy/dev/runner.Dockerfile` and given a `runner_pi_config:/home/runner/.pi` volume — that is Task 4. Confirmed via `runner-entrypoint.sh:180`, the Dockerfile npm block, and the `runner_{claude,codex,gemini}_config` volumes.
- **Token usage** is persisted to the backend `token_usages` table via gRPC (`backend/internal/service/tokenusage/service.go`); the runner log is only the collection smoke-signal. Task 5 verifies the DB.
- **pi session usage schema** (verified from a real session JSONL): `type:"message"` records carry `message.model` + `message.usage{input,output,cacheRead,cacheWrite}`. Map `cacheWrite→cacheCreation`. Sum across assistant messages.
- **Frontend needs zero changes** — agents are auto-discovered from the API (`AgentSelect.tsx` is data-driven).

### Deferred / out-of-scope (do NOT scope-creep into Session 2):
- **ACP mode** (`pi --mode rpc`) — Phase 2; its protocol is custom/undocumented, not standard ACP.
- **MCP injection** into pi pods (`MergeConfig`) — Phase 2.
- **Production / self-hosted runner** packaging — users install pi on their own runner host (distroless prod image has no npm). This plan is dev-runner only.
- **runner-2 `.pi` auth seeding** — declared the volume but did not seed it (test-only container, no pi e2e in scope).

## Known environment state

- pi pkg present on host: `~/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent` (bin `{"pi":"dist/cli.js"}`).
- Host `~/.pi/agent/auth.json` present (openai-codex OAuth) — confirm freshness before Task 4 seeding.
- Runner container runs as `user: "1000:1000"`; `docker compose exec ... chown` MUST use `--user root` (already baked into Task 4 Step 5).
- Latest migration is `000159`; `000160` is free.

## Fail-modes to anticipate (pre-cataloged response per mode)

1. **gazelle adds a wrong/missing dep** — `bazel build //runner/...` fails on undeclared dep. **Spec response:** fix the Go import (not the BUILD file), re-run `bazel run //:gazelle`. Never hand-edit BUILD deps.
2. **Contract test fails "registered parser, no fixture"** — blank import in `agents_import.go` missing, or import path wrong. **Spec response:** verify `register.go` registers `"pi"` and the blank import path is exact, re-run gazelle.
3. **pi pod launches but returns no completion** (Task 5) — expired/absent OAuth token in the container. **Spec response:** refresh `pi --provider openai-codex` locally (browser OAuth), re-seed `auth.json` into the volume via `docker compose cp` + `chown --user root`.
4. **Write landed in main repo, `git status` shows nothing to commit** — absolute path missing the worktree segment. **Spec response:** `mv` the file into the worktree path, re-commit; see [[worktree-write-path-gotcha]].
5. **agentfile eval error on `pi-cli`** (backend log) — DSL syntax drift. **Spec response:** diff against the live codex row: `psql ... SELECT agentfile_source FROM agents WHERE slug='codex-cli';`.
