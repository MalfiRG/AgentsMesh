# Lean Pi Pod Agent Design

Status: Rev 2 after adversarial review - blocked before implementation planning.
Review report: `docs/superpowers/reviews/2026-06-24-pi-lean-pod-agent-adversarial-review.md`.
Provenance: this file is the Rev 2 output edited in this run.

## Goal

Add a safe opt-in path for AgentsMesh pods to launch lean Pi, without changing the existing `pi-cli` behavior and without creating live pods during planning.

## Approved direction

Use Approach A: validate a separate lean Pi path with an AgentFile-layer prototype before any durable migration, backend rebuild, deployment, or live pod creation.

`pi-cli` remains the rollback path. The prototype is scoped to a single pod request through `agentfile_layer`; it does not mutate the builtin `pi-cli` row.

## Evidence

- Existing Pi agent is `pi-cli`, with `launch_command='pi'`. Verified: `grep -n "VALUES ('pi-cli'" backend/migrations/000160_add_pi_agent.up.sql` -> line 15.
- Existing Pi pod home is `sandbox.root + "/pi-home"`. Verified: `grep -n "ENV PI_CODING_AGENT_DIR" backend/migrations/000160_add_pi_agent.up.sql` -> line 16.
- `pi-pod-lean` reads `PI_POD_LEAN_BASE_DIR`. Verified: `grep -n "PI_POD_LEAN_BASE_DIR" /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` -> line 4.
- `pi-pod-lean` defaults to host-local profile state unless `PI_POD_LEAN_PROFILE_DIR` is set. Verified: `grep -n "PI_POD_LEAN_PROFILE_DIR" /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` -> line 5.
- `pi-pod-lean` reads named skill opt-ins from `PI_POD_SKILLS`. Verified: `grep -n "EXTRA_SKILLS" /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` -> lines 7 and 182.
- `pi-pod-lean --dry-run` is not read-only: `prepare_profile` runs before the dry-run print. Verified: `grep -n "prepare_profile" /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` -> lines 98 and 200, and `grep -n "DRY_RUN" ...` -> line 212.
- `pi-pod-lean` switches runtime Pi to `PROFILE_DIR`. Verified: `grep -n "export PI_CODING_AGENT_DIR" /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` -> line 221.
- AgentFile `AGENT` becomes `LaunchCommand`. Verified: `grep -n "LaunchCommand string" agentfile/eval/context.go` -> line 11, and `grep -n "ctx.Result.LaunchCommand = d.Command" agentfile/eval/eval_decl.go` -> line 10.
- AgentFile `EXECUTABLE` becomes executable metadata. Verified: `grep -n "Executable string" agentfile/eval/context.go` -> line 13, and `grep -n "ctx.Result.Executable = d.Name" agentfile/eval/eval_decl.go` -> line 12.
- Backend AgentFile eval keeps `LaunchArgs` separate from `Prompt` and `PromptPosition`. Verified: `grep -n "LaunchArgs" backend/internal/service/agent/config_builder_eval.go` -> line 81 and `grep -n "PromptPosition" ...` -> line 88.
- Runner injects the prompt after backend eval. Verified: `grep -n "PromptPosition" runner/internal/runner/pod_builder_build.go` -> lines 75-80.
- Agent home copying is env-var based through `agentkit.MatchAgentHome`, not slug based. Verified: `grep -n "MatchAgentHome" runner/internal/agentkit/home.go` -> lines 23-28 and `grep -n "MatchAgentHome" runner/internal/runner/pod_builder_agent_home.go` -> line 20.
- Current Pi token parser reads only `<sandbox>/pi-home/sessions`. Verified: `grep -n "sessionsDir := filepath.Join" runner/internal/agents/pi/parser.go` -> line 41.
- Current Pi parser aliases are only `pi` and `pi-cli`. Verified: `grep -n "RegisterParser" runner/internal/agents/pi/register.go` -> line 9.
- Token collection keys parser lookup by the pod agent, which is the launch command in the runner. Verified: `grep -n "Agent:" runner/internal/runner/pod_builder_build.go` -> line 187 and `grep -n "tokenusage.Collect" runner/internal/runner/handler_events_usage.go` -> line 26.
- Parser lookup normalizes absolute launch paths to their basename. Verified: `grep -n "LastIndexAny" runner/internal/tokenusage/parser_registry.go` -> line 32.
- `CreatePodRequest` exposes `agentfile_layer`, but no pod-label field in the create request. Verified: `grep -n "agentfile_layer" proto/pod/v1/pod.proto` -> line 243.

## Review caveat corrections

- Agent home copying is triggered by the `PI_CODING_AGENT_DIR` env var through `agentkit.MatchAgentHome`; it is not triggered by a `pi-lean-cli` slug. Parser aliases are still required for token accounting because token collection keys off the actual launch command.
- `PI_POD_LEAN_BASE_DIR` is implemented by the wrapper at line 4. The design must cite it and must set it to the copied pod home for the prototype.

## Non-goals

- Do not replace `pi-cli` in this phase.
- Do not rely on ticket labels until a source path proves labels flow into pod env or AgentFile.
- Do not create a live pod, write DB state, rebuild backend or web, deploy, or run live AgentsMesh operations during planning.
- Do not run the first live validation on the Hetzner runner.
- Do not treat lean Pi as a credential sandbox. It reduces startup context; it still inherits the configured Pi capability surface.

## Architecture

### Runtime shape

`pi-lean-cli` is a separate agent selection whose launch command is either the preflighted absolute wrapper path or `pi-pod-lean` under an explicit runner service `PATH`. It keeps `pi-cli` available as a rollback path.

The prototype uses `agentfile_layer` to override launch identity for one pod request. The durable migration may add a builtin `pi-lean-cli` row only after the prototype and parser contract pass.

### Canonical full AgentFile prototype

The laptop prototype pins the wrapper path after Phase 0 preflight. If the service `PATH` preflight is chosen instead, replace the quoted `AGENT` value with `pi-pod-lean` and keep the rest unchanged.

```agentfile
AGENT "/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean"
EXECUTABLE pi-pod-lean
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"
ENV HOME = sandbox.root + "/home"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"
ENV PI_POD_SKILLS = "agentsmesh"
PROMPT_POSITION append
REMOVE arg "--provider"
REMOVE arg "openai-codex"
REMOVE arg "--model"
REMOVE arg "gpt-5.5"
REMOVE arg "gpt-5.4"
REMOVE arg "gpt-5.3-codex-spark"
arg "--skill" "agentsmesh"
arg "--"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
```

Argument contract:

- Wrapper args come before the first `--`; Pi runtime args come after it.
- The second `--` terminates Pi runtime options before the appended prompt. A prompt beginning with `--skill`, `--base-dir`, or any future flag stays prompt text.
- The `REMOVE arg` lines make the layer safe when merged over the existing `pi-cli` AgentFile. If backend eval cannot produce the exact final sequence below, the prototype is blocked until the layer becomes a full replacement source or merge semantics gain a bulk arg reset.
- Required final args after eval and runner prompt injection: `--skill agentsmesh -- --provider openai-codex --model <model> -- <prompt>`.

### Pod-local env lifecycle

There are two `PI_CODING_AGENT_DIR` states:

- Pre-exec pod env: `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home`. Runner sees this in `CreatePodCommand.EnvVars` and copies host `.pi/agent` into the sandbox path through `agentkit.MatchAgentHome`.
- Runtime Pi env after wrapper exec: `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-lean-home`. The wrapper sets this by exporting `PI_CODING_AGENT_DIR="$PROFILE_DIR"` before `exec pi`.

`PI_POD_LEAN_BASE_DIR={{sandbox_root}}/pi-home` tells the wrapper to resolve skills and config from the copied Pi home. `PI_POD_LEAN_PROFILE_DIR={{sandbox_root}}/pi-lean-home` keeps lean runtime writes pod-local.

### Containment requirements

- Run `realpath` audits for `<sandbox>/pi-home`, `<sandbox>/pi-lean-home`, explicit skill paths, and parser session files.
- Fail if a copied or generated symlink resolves outside `sandbox.root`.
- Reject explicit skill paths that are absolute host paths, except for local debugging outside the recommended prototype. Named opt-in is the default.
- Skip parser inputs that are symlinks or non-regular files, and reject any parser path whose realpath escapes `sandbox.root`.
- Enforce parser scan limits: maximum file count, maximum single-file size, and maximum total bytes per pod session scan.

### Skills

Use named skill opt-in by default: `PI_POD_SKILLS=agentsmesh` or wrapper arg `--skill agentsmesh`. Do not recommend `/home/.../agentsmesh/SKILL.md` as the prototype path.

A dry-run or fake-run test must capture each resolved skill path and assert it is under `<sandbox>/pi-home`, `<sandbox>/pi-lean-home`, or another sandbox-owned directory. If the wrapper falls back to `$HOME/.agents/skills`, the pod env must set `HOME={{sandbox_root}}/home` and the resolved path must still be sandbox-contained.

`PI_POD_LABELS=pi-skills:agentsmesh` remains deferred until labels are proven to reach pod env or AgentFile data.

### Token usage and session artifacts

Durable support requires all runtime keys that can reach `tokenusage.Collect`:

- Register parser aliases for `pi`, `pi-cli`, `pi-pod-lean`, `pi-lean-cli`, and the basename of the approved absolute wrapper path.
- Test `tokenusage.Collect(<actual LaunchCommand>, sandboxFixture, startedAt)`, not only `GetParser`.
- Scan both `<sandbox>/pi-home/sessions` and `<sandbox>/pi-lean-home/sessions`; keep the existing path for `pi-cli` compatibility.
- Treat `{{sandbox_root}}/pi-lean-home` as a contract in parser tests, or update collection to read the effective profile path from redacted pod daemon metadata.
- Audit non-token session consumers before durable migration: MemPalace save hooks, precompact hooks, handoff or recovery tooling, and any context-audit reader that currently assumes `pi-home/sessions` only.

## Validation plan

### Phase 0: Non-live local checks and wrapper preflight

Use `mktemp -d` sandbox dirs and remove them on exit. `pi-pod-lean --dry-run` still prepares profile dirs and symlinks, so Phase 0 is non-live, not read-only.

- Decide wrapper path before Phase 1: either a preflighted absolute path or an explicit runner service `PATH` entry.
- For the absolute path, verify owner, group, mode, executable bit, symlink target, realpath containment for the chosen install root, and hash. Record the hash in the implementation plan.
- For the service `PATH` option, verify the runner service environment resolves `pi-pod-lean` to the expected path without relying on an interactive shell.
- Run dry-run with sandbox-local `PI_POD_LEAN_BASE_DIR`, `PI_POD_LEAN_PROFILE_DIR`, `HOME`, and explicit named skill opt-in.
- Confirm the dry-run command starts with `PI_CODING_AGENT_DIR=<sandbox>/pi-lean-home pi --no-skills`.
- Confirm resolved skill paths are sandbox-contained.
- Clean every `mktemp` directory before Phase 0 exits.

### Phase 1: Backend AgentFile eval tests

Add or extend backend AgentFile evaluation tests so the prototype layer produces:

- `LaunchCommand` equal to the approved wrapper path or `pi-pod-lean`.
- `Executable` equal to `pi-pod-lean`.
- `EnvVars.PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home`.
- `EnvVars.PI_POD_LEAN_BASE_DIR={{sandbox_root}}/pi-home`.
- `EnvVars.PI_POD_LEAN_PROFILE_DIR={{sandbox_root}}/pi-lean-home`.
- `PromptPosition=append` and `LaunchArgs` exactly matching the delimiter contract before runner prompt injection.
- Empty and non-empty `config.model` cases.

### Phase 2: Non-live runner seam and parser tests

Use fake home plus fake PTY or poddaemon seams. Do not create live pods.

- Final process args include wrapper delimiter, Pi runtime args, Pi prompt delimiter, and appended prompt in that order.
- A prompt beginning with `--skill` remains after the Pi prompt delimiter.
- Pre-exec daemon env contains `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home`.
- Agent-home copy creates `<sandbox>/pi-home` from fake host `.pi/agent`.
- Wrapper fixture or fake-run creates `<sandbox>/pi-lean-home` and session files.
- Pod daemon state shape records command, args, workdir, sandbox path, and redacted env without raw auth tokens.
- Parser aliases cover the actual `LaunchCommand`; `tokenusage.Collect` returns non-zero usage from `pi-lean-home/sessions` and still works for `pi-home/sessions`.
- Parser rejects symlink escapes, non-regular files, over-limit files, and over-limit session trees.

### Phase 3: Gated live laptop validation

Only after the operator types `fire create-pi-lean-pod`:

- Run on the laptop runner only. Pin explicit `runner_id`; disable auto-selection. Hetzner is excluded from first validation.
- Create exactly one non-perpetual pod with a fixed alias and the lean AgentFile layer.
- Inspect daemon state through a redacted script only. Do not print raw env, auth tokens, or full config files.
- Verify command, args, pre-exec env, workdir, `<sandbox>/pi-home`, and `<sandbox>/pi-lean-home/sessions`.
- Verify token usage is collected for the pod through the actual launch key.
- Verify context reduction with more than visible skill count: MCP servers, tool schemas, AGENTS.md, extensions, prompts, themes, selected tools, and input-token count.
- Teardown the pod. If a check fails or the pod gets stuck, terminate by pod key, remove the sandbox dirs, and inspect only redacted logs.

## Rollback

Prototype rollback:
- Stop using the AgentFile layer and select `pi-cli`.
- Remove any local wrapper path override or service `PATH` test change made for the prototype.
- Delete only the prototype `mktemp` dirs and any one-off sandbox dirs created by the gated live test.

Durable migration rollback:
- Disable or remove only the future `pi-lean-cli` agent row.
- Revert parser aliases or wrapper install only if no running pod uses them; otherwise leave aliases inert until pods drain.
- Revert runner service `PATH` or wrapper installation through the same package, symlink, and hash record used during rollout.
- Do not alter `pi-cli` as part of lean-path rollback.

## Open decisions for implementation planning

1. Durable launch command: bare `pi-pod-lean` with runner service `PATH`, or an installed absolute path contract.
2. Wrapper ownership: runner prerequisite, vendored executable, or installed path managed by pi-config.
3. Session artifact contract: fixed `pi-lean-home/sessions` path, or metadata-driven profile discovery.
4. Label-to-skill mapping: backend feature, AgentFile convention, or deferred until labels expose a pod config path.
5. Live validation runner: laptop only for first validation; Hetzner remains excluded until the lean path has passed laptop validation.
