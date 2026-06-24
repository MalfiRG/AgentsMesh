# Lean Pi Pod Agent Design - Adversarial Review Report

## Scope

Reviewed artifact: `docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md`

Run IDs and coverage:

- Taskflow run: `pi-lean-pod-agent-advers-mqrb8p22-978e59`
- Forward reviewers completed: adversarial TL, architecture, security, consistency, Socratic, external truth
- Taskflow Claude model override failed: Anthropic provider unavailable in Pi
- Cross-model fallback completed via Antigravity Claude Sonnet 4.6 Thinking

No files were edited by reviewers. No pod was created. No DB state, backend image, web image, or deployment was touched.

## Verdict

BLOCKED before implementation planning.

The design direction is still sound, but the spec needs Rev 2 before an implementation plan. The blockers are mostly about the prototype not being explicit enough about the AgentFile layer, wrapper argument boundary, sandbox containment, token/session visibility, and live-pod validation safety.

## Reviewer severity summary

| Reviewer | Critical | High | Medium | Low |
|---|---:|---:|---:|---:|
| Adversarial TL | 2 | 6 | 2 | 0 |
| Architecture | 0 | 2 | 4 | 1 |
| Security | 0 | 4 | 4 | 0 |
| Consistency | 0 | 2 | 3 | 0 |
| External truth | 0 | 0 | 1 | 0 |
| Claude cross-model | 2 | 5 | 6 | 4 |

Deduped blocking themes: 8.

## Cross-model caveats

The Claude cross-model pass surfaced useful independent findings, but two items need correction before applying blindly:

- `F-XMOD-02` is partly false. Runner agent-home copying is env-var based through `agentkit.MatchAgentHome`, not slug based. Parser aliases are still needed, but the copy trigger does not require a `pi-lean-cli` slug registration.
- `F-XMOD-05` is not a real uncertainty in the repo. `pi-pod-lean` line 4 does read `PI_POD_LEAN_BASE_DIR`. The spec should cite that line, but the env var is implemented.

## Must-fix findings for Rev 2

### 1. Add a canonical full AgentFile prototype

Source IDs: `F-ADV-01`, `F-ARCH-01`, `Q-02`

The spec says Phase 1 should produce `LaunchCommand=pi-pod-lean`, but the shown prototype only sets env and args. Base `pi-cli` still has `AGENT pi`. AgentFile eval maps `AGENT` to `LaunchCommand`, so the spec must show the full layer, including the command override.

Required edit:

- Add a complete prototype AgentFile block.
- Include `AGENT pi-pod-lean` or the approved absolute wrapper path.
- Include `EXECUTABLE pi-pod-lean` if the UI/schema needs executable metadata.
- Keep `pi-cli` untouched.

### 2. Fix prompt and wrapper argument boundary semantics

Source IDs: `F-ADV-02`, `F-ADV-03`, `F-SEC-02`, `F-CONS-05`, `F-XMOD-01`, `F-XMOD-15`

Backend eval cannot observe the final prompt in `LaunchArgs`; runner appends it later. The spec also puts Pi args before the wrapper delimiter, so wrapper option injection is possible if a future value matches a wrapper flag.

Required edit:

- State the two lifecycle states explicitly:
  - Pre-exec pod env: `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home`, used by runner to copy Pi home.
  - Runtime Pi env after wrapper exec: `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-lean-home`, set by the wrapper.
- Move the delimiter before Pi runtime args in the prototype if possible: wrapper args first, then `--`, then `--provider`, `--model`, and prompt.
- Add backend eval tests for `LaunchArgs` and `PromptPosition`.
- Add runner-level tests for final process args, including a prompt beginning with `--skill`.
- Avoid duplicate inherited provider/model args, or assert the exact expected sequence.

### 3. Remove host absolute skill paths from the recommended path

Source IDs: `F-ADV-05`, `F-ARCH-03`, `F-SEC-03`, `F-CONS-02`, `F-XMOD-03`, `Q-03`

The spec labels `/home/malfirg/.claude/skills/agentsmesh/SKILL.md` as the safest option. That contradicts the sandbox-local invariant and overfits the laptop.

Required edit:

- Make `PI_POD_SKILLS=agentsmesh` or `--skill agentsmesh` the prototype default.
- Require proof that resolved skill paths stay under the sandbox copied Pi home, or mark host absolute paths as local-only debugging, not the spec default.
- Add a realpath containment check for every resolved explicit skill path.

### 4. Resolve wrapper availability before any live pod

Source IDs: `F-ADV-04`, `F-ARCH-04`, `F-SEC-05`, `F-XMOD-04`, `Q-01`

The spec leaves bare `pi-pod-lean` vs absolute path open. Runner executes `LaunchCommand` directly, and the laptop runner service currently has no explicit PATH.

Required edit:

- Make wrapper resolution a Phase 0 prerequisite.
- Either pin an absolute wrapper path for the prototype, or require an explicit runner service PATH preflight.
- For durable migration, define wrapper ownership: runner prerequisite, vendored executable, or installed path contract.
- Add ownership, permission, symlink, and hash checks for the wrapper if using an absolute path.

### 5. Add symlink and realpath containment audits

Source IDs: `F-ADV-06`, `F-SEC-01`, `F-SEC-07`, `F-XMOD-06`, `F-XMOD-09`

Sandbox-local env vars are not enough if copied Pi home or lean profile entries are symlinks that resolve outside the sandbox.

Required edit:

- Require realpath audits for `<sandbox>/pi-home` and `<sandbox>/pi-lean-home`.
- Fail if any copied or generated symlink resolves outside `sandbox.root`.
- For parser scanning, skip non-regular files and reject symlinks or paths that escape `sandbox.root`.
- Add file count, file size, and total byte limits for session parsing.

### 6. Strengthen token and session artifact contract

Source IDs: `F-ADV-10`, `F-ARCH-06`, `F-CONS-03`, `F-XMOD-07`, `Q-04`, `Q-05`

The spec mentions token parser visibility, but it does not fully define the durable session artifact contract.

Required edit:

- Register parser aliases for all runtime keys that can be observed: `pi`, `pi-cli`, `pi-pod-lean`, `pi-lean-cli`, and the approved absolute-path basename if applicable.
- Test `tokenusage.Collect(<actual LaunchCommand>, sandboxFixture, startedAt)`, not only `GetParser`.
- Treat `{{sandbox_root}}/pi-lean-home` as a durable contract, or update token collection to read the effective profile path from pod daemon metadata.
- Identify whether MemPalace/precompact/session recovery hooks also need to read `pi-lean-home/sessions`.

### 7. Add a runner build seam before live pod validation

Source IDs: `F-ADV-02`, `F-ARCH-05`, `Q-09`

Current Phase 1 covers backend eval, and Phase 3 covers live inspection. Missing is a non-live runner-level test for placeholder resolution, agent-home copy, final args, env capture, and daemon state shape.

Required edit:

- Add a Phase 2 or Phase 3 dry runner test using fake home and fake PTY/poddaemon.
- Assert `pi-home` copy occurs.
- Assert `pi-lean-home` profile is created by wrapper or fixture.
- Assert final args include `--` before the prompt.
- Assert daemon env contains pre-exec `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home`.

### 8. Make live pod validation laptop-only, redacted, and self-cleaning

Source IDs: `F-ADV-08`, `F-ADV-09`, `F-SEC-04`, `F-SEC-08`, `F-XMOD-11`, `F-XMOD-12`, `Q-07`, `Q-08`

The live phase is gated, but still underspecified.

Required edit:

- Phase 3 must pin the laptop runner by explicit `runner_id`; no auto-selection.
- Exclude Hetzner from the first live lean-pod validation.
- The token `fire create-pi-lean-pod` authorizes one laptop pod only.
- Inspect `pod_daemon.json` through a redacted script; never print raw env or auth token.
- Use a fixed alias, non-perpetual pod, and a teardown step.
- Define stuck-pod and failed-check cleanup.

## Nice-to-fix findings

- Context audit success should measure more than visible skill count. Include MCP servers/tool schemas, AGENTS.md, extensions, prompts, themes, selected tools, and input tokens.
- Clarify that `pi-pod-lean --dry-run` is not read-only because it prepares the profile before printing. Rename Phase 0 to non-live local checks and use `mktemp -d` with cleanup.
- Reserve the `pi-lean-cli` slug before durable migration to avoid collision.
- Add tests for `config.model` empty and non-empty cases.
- Split rollback into prototype-layer rollback and durable-agent-row rollback.

## Recommended Rev 2 edits by section

### Evidence

Add citations for:

- `PI_POD_LEAN_BASE_DIR` in the wrapper.
- AgentFile `AGENT` to `LaunchCommand` behavior.
- Backend eval prompt separation vs runner prompt injection.
- `agentkit.MatchAgentHome` env-var matching.

### Runtime shape

Add the full AgentFile prototype and distinguish prototype command path from durable command path.

### Pod-local directories

Add pre-exec vs post-exec env lifecycle and realpath containment rules.

### Prompt and wrapper argument boundary

Replace the current snippet with a delimiter-safe, merge-aware snippet. Add exact arg-sequence tests.

### Skills

Remove the host absolute path as the recommended option. Make named skill opt-in the default and require sandbox-contained resolution.

### Token usage and compaction artifacts

Add parser alias and collector tests for the actual launch command. Add session consumer audit beyond token usage.

### Validation plan

Rename Phase 0, add wrapper preflight, add runner seam tests, add redacted daemon inspection, pin laptop runner, add teardown.

### Rollback

Split prototype rollback from durable migration rollback.

## Next action

Apply Rev 2 to the spec before writing the implementation plan.
