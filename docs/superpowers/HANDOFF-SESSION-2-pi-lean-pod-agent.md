# Session 2 Handoff - AgentsMesh lean Pi pod agent

**Status at session open:** Repo `main` at `aae3747d5` before this handoff commit. Current working tree has untracked lean-Pi spec and review docs, plus unrelated dirty files listed below. Spec Rev 2 exists; implementation plan is not written yet.
**Primary path for Session 3:** Write `docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md` with the writing-plans skill, then run the same adversarial review rigor on the plan before implementation.
**Blocker class:** Planning not complete. No code, DB, pod, rebuild, deploy, or live validation has happened for lean Pi pods.

---

## Bootstrap prompt (paste into a fresh session)

```text
Resume the AgentsMesh lean Pi pod agent pipeline at implementation-plan writing.

**Worktree:** `/home/malfirg/programming_projects/AgentsMesh` (branch `main`, pre-handoff SHA `aae3747d5`; this handoff commit sits one above it)
**Status:** Spec Rev 2 exists and is hardened by adversarial review. The implementation plan file is still missing. Do not implement before writing and reviewing the plan.

**Load-bearing docs (read in order):**
1. `docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md` (199 lines, 14696 bytes) - Rev 2 design, current source of truth.
2. `docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md` (202 lines, 9384 bytes) - review findings that drove Rev 2.
3. `docs/superpowers/HANDOFF-SESSION-2-pi-lean-pod-agent.md` - this handoff.
4. Source files cited by the spec before planning exact tasks:
   - `backend/migrations/000160_add_pi_agent.up.sql`
   - `backend/internal/service/agent/config_builder_agentfile.go`
   - `backend/internal/service/agent/config_builder_eval.go`
   - `agentfile/eval/eval_decl.go`
   - `agentfile/eval/apply_removes.go`
   - `runner/internal/agentkit/home.go`
   - `runner/internal/agents/pi/register.go`
   - `runner/internal/agents/pi/parser.go`
   - `runner/internal/agents/pi/testsupport/fixture.go`
   - `runner/internal/tokenusage/parser_registry.go`
   - `runner/internal/tokenusage/collector.go`
   - `runner/internal/runner/pod_builder_build.go`
   - `runner/internal/runner/pod_builder_agent_home.go`
   - `runner/internal/runner/pod_builder_agent_home_helpers.go`
   - `runner/internal/poddaemon/state.go`
   - `/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean`

**Recovery if context is thin after compaction:**
Run `mempalace_search("AgentsMesh lean Pi pod agent pi-lean-cli implementation plan")`, then read the two docs above directly.

**Bootstrap sanity checks with expected output:**
1. `git -C /home/malfirg/programming_projects/AgentsMesh log --oneline -2` - expect this handoff commit above `aae3747d5`.
2. `git -C /home/malfirg/programming_projects/AgentsMesh branch --show-current` - expect `main`.
3. `git -C /home/malfirg/programming_projects/AgentsMesh status --short docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md` - expect spec and review untracked unless already staged later; expect plan missing before you write it.
4. `wc -l docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md` - expect `199` and `202` lines.
5. `wc -c docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md` - expect `14696` and `9384` bytes unless the docs were edited after this handoff.
6. `test -f docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md && echo PLAN_EXISTS || echo PLAN_MISSING` - expect `PLAN_MISSING` at handoff time.
7. `command -v claude && claude --version` - expect `/home/malfirg/.local/bin/claude` and `2.1.187 (Claude Code)` or newer.

**Next-session path:**
1. Use the writing-plans skill to create `docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md`.
2. Plan should be scoped to non-live implementation first: backend AgentFile eval tests, runner seam tests, parser aliases and session scanning, containment checks, preflight/check scripts or docs, and redacted validation tooling. No live pod in the plan execution until the explicit token.
3. After the plan is written, run adversarial review on the plan with plan-level rigor: adversarial TL, architecture, security, consistency, Socratic, traceability, and external-truth reviewers.
4. For the cross-model pass, use the installed headless Claude binary instead of Pi taskflow Anthropic model override. The Pi override failed due missing Anthropic provider, but host Claude works.

**Headless Claude cross-model command pattern:**
`claude -p --model sonnet --permission-mode plan --add-dir /home/malfirg/programming_projects/AgentsMesh "<review prompt>"`

Use Claude only for review/arbitration unless the operator explicitly asks it to implement.

**Safety constraints:**
- Invoke the `agentsmesh` skill first for any AgentsMesh work.
- MemPalace first after compaction, before broad grep or web.
- Do not create a live pod, write DB state, rebuild backend/web, deploy, or run live AgentsMesh operations before the operator types `fire create-pi-lean-pod`.
- The first live lean-pod validation, when eventually authorized, must be laptop-runner only with explicit `runner_id`, one non-perpetual pod, redacted daemon inspection, and teardown.
- Do not run on Hetzner for first validation. It co-hosts ScoutQL production.
- Never `docker compose pull` backend or web in this self-hosted stack.
- PR or push to official upstream requires `fire pr-official`; current normal pipeline is local to fork.
- Use ASCII hyphen only. No long dash codepoints.

**Model tiering:** Pi/Codex = plan and code executor; Claude Sonnet headless = cross-model review; Opus only for arbitration if explicitly available.

After bootstrap, write the plan. Do not implement yet. After plan writing, run adversarial review and cross-model review before offering execution.
```

---

## Settled, do NOT re-discuss

### Closed in this session

- **Approach A accepted:** separate lean path first; do not mutate `pi-cli`.
- **Spec Rev 2 applied:** `docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md` is the source of truth, 199 lines and 14696 bytes at handoff.
- **Review report written:** `docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md`, 202 lines and 9384 bytes at handoff.
- **Cross-model note:** Pi taskflow Anthropic model override failed due missing provider config. Host `claude` binary is installed and should be used for future Claude Sonnet headless reviews.
- **No implementation plan yet:** `docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md` is absent at handoff.

### Design invariants carried into the plan

- AgentFile prototype must include a real `AGENT` launch identity override. Env-only snippets are insufficient.
- Pre-exec `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home` is for runner copy. Runtime Pi sees `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-lean-home` after wrapper exec.
- `PI_POD_LEAN_BASE_DIR={{sandbox_root}}/pi-home` and `PI_POD_LEAN_PROFILE_DIR={{sandbox_root}}/pi-lean-home` are sandbox-local requirements.
- Named skill opt-in (`PI_POD_SKILLS=agentsmesh` or `--skill agentsmesh`) is preferred. Host absolute skill paths are not the recommended path.
- Realpath containment is required for copied Pi home, lean profile entries, explicit skill paths, and parser session files.
- Token usage must cover actual launch keys and both `pi-home/sessions` and `pi-lean-home/sessions`.
- Non-live runner seam tests are required before any live pod validation.
- Live validation is gated by `fire create-pi-lean-pod`, laptop-runner only, redacted, and self-cleaning.

### Deferred or fair game

- Durable `pi-lean-cli` DB migration is deferred until prototype, parser, and runner seams pass.
- Ticket label to skill mapping is deferred until labels are proven to reach pod env or AgentFile data.
- Hetzner runner validation is out of scope for the first live test.
- Production deployment, backend image rebuild, and web image rebuild are out of scope until separately authorized.

## Known environment state

- Repo: `/home/malfirg/programming_projects/AgentsMesh`, branch `main`, pre-handoff HEAD `aae3747d5`.
- Dirty state before handoff commit:
  - `M .claude/settings.local.json`
  - untracked `.codex-reviews/20260614-102231-cross-model.md`
  - untracked `.codex-reviews/20260619-155135-ticket-labels-cross-model.md`
  - untracked `docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md`
  - untracked `docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md`
  - untracked `memory/`
- Existing prior handoff: `docs/superpowers/HANDOFF-SESSION-1-pi-agent.md`.
- No `SESSION-CHECKPOINTS.md` found in this repo.
- AgentsMesh runner service is active.
- Self-host Docker stack containers were running; backend and web reported healthy at handoff check.
- `claude` binary: `/home/malfirg/.local/bin/claude`, version `2.1.187 (Claude Code)` at handoff.

## Fail-modes to anticipate

1. **Taskflow Claude override fails again** - symptom: `No API key found for anthropic`. **Response:** run cross-model review with `claude -p --model sonnet --permission-mode plan --add-dir /home/malfirg/programming_projects/AgentsMesh "<prompt>"`.
2. **Plan drifts into live pod creation** - symptom: plan tasks include ext API pod creation before validation tasks. **Response:** split live pod into a gated final validation appendix and require `fire create-pi-lean-pod`.
3. **Plan assumes `pi-pod-lean --dry-run` is read-only** - symptom: Phase 0 says read-only dry-run. **Response:** fix wording to non-live local check with `mktemp -d` and cleanup, because wrapper prepares profile before printing.
4. **Plan relies on host absolute skill paths** - symptom: `/home/malfirg/.claude/skills/agentsmesh/SKILL.md` appears as recommended implementation. **Response:** replace with named skill opt-in and sandbox realpath checks.
5. **Plan omits runner seam tests** - symptom: only backend eval and parser tests exist before live validation. **Response:** add fake-home/fake-PTY or poddaemon seam tests for final args, env, agent-home copy, and daemon state shape.
6. **Plan misses token collection key reality** - symptom: only `GetParser("pi-lean-cli")` is tested. **Response:** require `tokenusage.Collect(<actual LaunchCommand>, sandboxFixture, startedAt)` and absolute-path basename cases.
7. **Plan proposes Hetzner validation** - symptom: `runner_id` unspecified or Hetzner allowed. **Response:** pin laptop runner only and forbid auto-selection for first live test.

## Handoff write verification

At handoff creation time:

- Spec file existed: 199 lines, 14696 bytes.
- Review file existed: 202 lines, 9384 bytes.
- Plan file missing: `PLAN_MISSING`.
- Long dash scan across spec and review returned no matches.
- Claude binary present at `/home/malfirg/.local/bin/claude`.
- No live pod, DB write, rebuild, deploy, or live AgentsMesh operation was performed in this handoff step.
