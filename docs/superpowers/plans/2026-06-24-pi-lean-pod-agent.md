# Lean Pi Pod Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** Rev 3 - post-adversarial-and-cross-model review; ready for implementation approval.

**Review:** 3-agent round 1 found 4 blockers, 7 non-blocking fixes, and 3 optional/deferred items; Rev 2 applied those fixes. 3-agent round 2 found 3 targeted blockers; Rev 3 applies parser-helper, scan-shell, and daemon-argv-redaction fixes. 3-agent round 3 and headless Sonnet cross-model review passed with no blockers.

**Goal:** Build a non-live, opt-in AgentsMesh path that can launch lean Pi through an AgentFile layer, with parser, runner, containment, and validation seams proven before any live pod is created.

**Architecture:** Keep `pi-cli` unchanged and make the lean path a prototype layer first. Fix AgentFile merge semantics so a layer can remove base args and re-add safe wrapper args, then prove the merged command through backend eval, runner launch planning, Pi token parsing, and local validation scripts. Live validation remains a gated appendix, not part of normal plan execution.

**Tech Stack:** Go, Bazel, AgentFile parser/merge/eval/serialize packages, runner PTY pod builder, Pi token JSONL parser, Bash validation helpers.

## Global Constraints

- Do not create a live pod, write DB state, rebuild backend/web, deploy, or run live AgentsMesh operations before the operator types `fire create-pi-lean-pod`.
- Keep existing `pi-cli` behavior unchanged and available as rollback.
- Prototype through `agentfile_layer`; do not add a durable `pi-lean-cli` migration in this plan.
- Use named skill opt-in by default: `PI_POD_SKILLS=agentsmesh` or `--skill agentsmesh`.
- Pre-exec pod env must keep `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-home` so `agentkit.MatchAgentHome` copies host `.pi/agent` into the sandbox.
- Runtime wrapper env must set `PI_CODING_AGENT_DIR={{sandbox_root}}/pi-lean-home` through `PI_POD_LEAN_PROFILE_DIR`.
- Parser support must cover actual launch keys: `pi`, `pi-cli`, `pi-pod-lean`, `pi-lean-cli`, and basename lookup for an approved absolute wrapper path.
- Parser scanning must include both `<sandbox>/pi-home/sessions` and `<sandbox>/pi-lean-home/sessions`.
- Parser and copied/generated profile paths must reject symlink escapes and non-regular session inputs.
- First live validation, if later authorized, must pin the laptop runner by explicit `runner_id`; do not run Hetzner first.
- Do not print raw env, auth tokens, full `auth.json`, or full config files in validation output.
- Use ASCII hyphens only. No long dash codepoints.

---

## File map

### AgentFile merge and serialization

- Modify: `agentfile/merge/merge.go`
  - Make `REMOVE arg` consume matching base `arg` statements during merge, before layer statements are appended.
  - Do not keep consumed arg removes in the serialized merged program; otherwise `eval.ApplyRemoves` removes the layer's re-added args too.
- Modify: `agentfile/merge/merge_test.go`
  - Add a regression test for removing base `--provider` and `--model`, then re-adding those flags in the layer.
- Modify: `agentfile/serialize/serialize_merge_test.go`
  - Add a round-trip test proving serialized merged source keeps the layer's re-added wrapper args and has no stale `REMOVE arg` declaration.

### Backend AgentFile extraction and config build

- Modify: `backend/internal/service/agentpod/agentfile_extract_test.go`
  - Add a pi lean extraction test using the real base `pi-cli` AgentFile shape plus the lean layer.
- Create: `backend/internal/service/agent/config_builder_pi_lean_test.go`
  - Add `ConfigBuilder` tests for the serialized lean merged source, including non-empty and empty model cases.
- Modify: `backend/internal/service/agent/BUILD.bazel`
  - Add `config_builder_pi_lean_test.go` to `agent_test` srcs.

### Pi parser and token collection

- Modify: `runner/internal/agents/pi/register.go`
  - Register aliases `pi`, `pi-cli`, `pi-pod-lean`, and `pi-lean-cli` on the same parser.
- Modify: `runner/internal/agents/pi/parser.go`
  - Scan both `pi-home/sessions` and `pi-lean-home/sessions`.
  - Add scan limits and realpath containment checks.
  - Skip symlinks, non-regular files, and files outside `sandboxPath` after realpath.
- Modify: `runner/internal/agents/pi/testsupport/fixture.go`
  - Add helper(s) to plant fixture JSONL under either session root.
- Modify: `runner/internal/agents/pi/parser_test.go`
  - Add tests for lean-home sessions, dual roots, symlink escape rejection, non-regular file rejection, max visited entries, max depth, max file count, max single-file size, and max total bytes.
- Modify: `runner/internal/tokenusage/parser_agents_test.go`
  - Import the Pi package for parser registration and add actual launch key cases.
- Modify: `runner/internal/tokenusage/parser_contract_test.go`
  - Import the Pi package for parser registration if not already covered after Task 3.
- Modify: `runner/internal/tokenusage/BUILD.bazel`
  - Add `//runner/internal/agents/pi` to `tokenusage_test` deps.

### Runner launch and containment seams

- Create: `runner/internal/runner/pod_builder_launch_plan.go`
  - Extract a pure launch-input resolver used by `Build`: command, resolved args, resolved env, and captured env.
- Modify: `runner/internal/runner/pod_builder_build.go`
  - Replace duplicated inline resolution logic with the helper.
- Create: `runner/internal/runner/pod_builder_launch_plan_test.go`
  - Assert lean wrapper args and prompt delimiter ordering without starting a real Pi pod.
- Create: `runner/internal/runner/agent_home_containment.go`
  - Validate the resolved agent home before copy, mkdir, merge, or cleanup, then validate the created tree after copy or mkdir.
- Modify: `runner/internal/runner/pod_builder_agent_home.go`
  - Call containment validation after creating or copying an agent home.
- Modify: `runner/internal/runner/pod_builder_agent_home_test.go`
  - Add symlink escape and contained symlink tests.
- Modify: `runner/internal/runner/BUILD.bazel`
  - Add the new production files and new test file.

### Non-live validation helpers

- Create: `tools/pi-lean/preflight.sh`
  - Non-live wrapper preflight using `mktemp -d`; validates wrapper path, owner/mode, hash, dry-run output, and skill path containment.
- Create: `tools/pi-lean/inspect-pod-daemon-redacted.sh`
  - Reads a sandbox path and prints only redacted daemon state fields.
- Create: `tools/pi-lean/BUILD.bazel`
  - Add concrete `sh_test` syntax targets for `preflight.sh` and `inspect-pod-daemon-redacted.sh`.

---

## Canonical lean AgentFile layer

Use this layer for backend extraction tests and for the later gated live request. The wrapper path is a test variable in unit tests; live use must be chosen by Phase 0 preflight.

```agentfile
AGENT "/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean"
EXECUTABLE pi-pod-lean
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark", "") = "gpt-5.5"
ENV HOME = sandbox.root + "/home"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"
ENV PI_POD_SKILLS = "agentsmesh"
PROMPT_POSITION append
REMOVE arg "--provider"
REMOVE arg "--model"
arg "--skill" "agentsmesh"
arg "--"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
```

Required post-eval `LaunchArgs` before runner prompt injection with default model:

```go
[]string{"--skill", "agentsmesh", "--", "--provider", "openai-codex", "--model", "gpt-5.5", "--"}
```

Required final runner args after appending prompt `build the thing`:

```go
[]string{"--skill", "agentsmesh", "--", "--provider", "openai-codex", "--model", "gpt-5.5", "--", "build the thing"}
```

Required final runner args when the prompt begins with a flag-like token:

```go
[]string{"--skill", "agentsmesh", "--", "--provider", "openai-codex", "--model", "gpt-5.5", "--", "--skill should stay prompt text"}
```

---

### Task 1: Fix AgentFile `REMOVE arg` merge semantics

**Files:**
- Modify: `agentfile/merge/merge.go`
- Modify: `agentfile/merge/merge_test.go`
- Modify: `agentfile/serialize/serialize_merge_test.go`

**Interfaces:**
- Consumes: existing `merge.Merge(base, slice)` behavior.
- Produces: `REMOVE arg` removes matching base `arg` statements before layer args are appended, and serialized merged output does not contain consumed `REMOVE arg` declarations.

**Compatibility note:** In this plan, `REMOVE arg` is a merge-time base-statement replacement mechanism. It matches base `arg` statements by first string literal only. The lean layer uses it only for `--provider` and `--model`, which are first literals in the existing `pi-cli` base AgentFile. The Task 1 tests must pin that limited scope so later work does not treat `REMOVE arg` as a general argv substring remover.

- [ ] **Step 1: Write the failing merge test**

Add this test to `agentfile/merge/merge_test.go`:

```go
func TestMerge_RemoveArgAllowsLayerToReAddSameFlag(t *testing.T) {
	base := parse(t, `
AGENT pi
CONFIG model SELECT("gpt-5.5", "gpt-5.4") = "gpt-5.5"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
`)
	layer := parse(t, `
REMOVE arg "--provider"
REMOVE arg "--model"
arg "--skill" "agentsmesh"
arg "--"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
`)

	Merge(base, layer)

	ctx := eval.NewContext(map[string]interface{}{
		"config": map[string]interface{}{"model": "gpt-5.5"},
	})
	require.NoError(t, eval.Eval(base, ctx))
	eval.ApplyRemoves(ctx.Result)

	assert.Equal(t, []string{
		"--skill", "agentsmesh",
		"--",
		"--provider", "openai-codex",
		"--model", "gpt-5.5",
		"--",
	}, ctx.Result.LaunchArgs)
}
```

- [ ] **Step 2: Run the focused merge test and verify it fails**

Run:

```bash
bazel test //agentfile/merge:merge_test --test_filter=TestMerge_RemoveArgAllowsLayerToReAddSameFlag
```

Expected before implementation: FAIL because post-eval remove semantics remove the layer's re-added `--provider` and `--model` args.

- [ ] **Step 3: Implement merge-time arg statement removal**

In `agentfile/merge/merge.go`, change `Merge` and `applySliceDeclarations` like this:

```go
func Merge(base, slice *parser.Program) {
	baseDecls := indexDeclarations(base.Declarations)

	base.Statements = applyArgRemovesToStatements(base.Statements, slice.Declarations)
	merged := applySliceDeclarations(baseDecls, slice.Declarations)

	base.Declarations = flattenDeclarations(merged)
	base.Statements = append(base.Statements, slice.Statements...)
}
```

In the `case *parser.RemoveDecl:` block, handle `arg` removes as consumed declarations:

```go
case *parser.RemoveDecl:
	if sd.Target == "arg" {
		continue
	}
	key := declKey{Type: sd.Target, Name: sd.Name}
	delete(base.decls, key)
	rKey := declKey{Type: "REMOVE", Name: sd.Target + "." + sd.Name}
	base.decls[rKey] = d
	base.order = append(base.order, rKey)
```

Add helpers near `unionStrings`:

```go
func applyArgRemovesToStatements(stmts []parser.Statement, decls []parser.Declaration) []parser.Statement {
	result := stmts
	for _, d := range decls {
		rd, ok := d.(*parser.RemoveDecl)
		if !ok || rd.Target != "arg" {
			continue
		}
		result = removeArgStatements(result, rd.Name)
	}
	return result
}

func removeArgStatements(stmts []parser.Statement, name string) []parser.Statement {
	filtered := make([]parser.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		arg, ok := stmt.(*parser.ArgStmt)
		if ok && argStartsWithLiteral(arg, name) {
			continue
		}
		filtered = append(filtered, stmt)
	}
	return filtered
}

func argStartsWithLiteral(arg *parser.ArgStmt, name string) bool {
	if len(arg.Args) == 0 {
		return false
	}
	lit, ok := arg.Args[0].(*parser.StringLit)
	return ok && lit.Value == name
}
```

- [ ] **Step 4: Add serialize regression coverage**

Add this test to `agentfile/serialize/serialize_merge_test.go`:

```go
func TestMergeSerialize_RemoveArgAllowsLayerToReAddSameFlag(t *testing.T) {
	base := parse(t, `
AGENT pi
CONFIG model SELECT("gpt-5.5", "gpt-5.4") = "gpt-5.5"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
`)
	layer := parse(t, `
REMOVE arg "--provider"
REMOVE arg "--model"
arg "--skill" "agentsmesh"
arg "--"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
`)

	merge.Merge(base, layer)
	src := Serialize(base)
	assert.NotContains(t, src, "REMOVE arg")

	reparsed := parse(t, src)
	ctx := eval.NewContext(map[string]interface{}{
		"config": map[string]interface{}{"model": "gpt-5.5"},
	})
	require.NoError(t, eval.Eval(reparsed, ctx))
	eval.ApplyRemoves(ctx.Result)
	assert.Equal(t, []string{
		"--skill", "agentsmesh",
		"--",
		"--provider", "openai-codex",
		"--model", "gpt-5.5",
		"--",
	}, ctx.Result.LaunchArgs)
}
```

- [ ] **Step 5: Run Task 1 tests**

Run:

```bash
bazel test //agentfile/merge:merge_test //agentfile/serialize:serialize_test
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

```bash
git add agentfile/merge/merge.go agentfile/merge/merge_test.go agentfile/serialize/serialize_merge_test.go
git commit -m "fix(agentfile): allow layers to replace launch args"
```

---

### Task 2: Prove backend lean AgentFile extraction and command build

**Files:**
- Modify: `backend/internal/service/agentpod/agentfile_extract_test.go`
- Create: `backend/internal/service/agent/config_builder_pi_lean_test.go`
- Modify: `backend/internal/service/agent/BUILD.bazel`

**Interfaces:**
- Consumes: Task 1 merge semantics and existing `extractFromAgentfileLayer` serialization.
- Produces: backend tests proving the lean layer generates the required command, executable, env vars, prompt position, and launch args before runner prompt injection.

- [ ] **Step 1: Add an extraction test for the lean layer**

Add imports to `backend/internal/service/agentpod/agentfile_extract_test.go`:

```go
import (
	"testing"

	"github.com/anthropics/agentsmesh/agentfile/eval"
	"github.com/anthropics/agentsmesh/agentfile/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

Add constants and the test near the existing extraction tests:

```go
const piBaseAgentfileSrc = `AGENT pi
EXECUTABLE pi
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
PROMPT_POSITION prepend
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
`

func piLeanLayer(wrapperPath string) string {
	return `AGENT "` + wrapperPath + `"
EXECUTABLE pi-pod-lean
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark", "") = "gpt-5.5"
ENV HOME = sandbox.root + "/home"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"
ENV PI_POD_SKILLS = "agentsmesh"
PROMPT_POSITION append
REMOVE arg "--provider"
REMOVE arg "--model"
arg "--skill" "agentsmesh"
arg "--"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
`
}

func TestExtractAgentfileOverrides_PiLeanLayerSerializesRunnableArgs(t *testing.T) {
	wrapper := "/opt/agentsmesh/bin/pi-pod-lean"
	result, err := extractFromAgentfileLayer(piBaseAgentfileSrc, piLeanLayer(wrapper), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "pty", result.Mode)
	assert.Equal(t, "gpt-5.5", result.ConfigValues["model"])

	prog, errs := parser.Parse(result.MergedAgentfileSource)
	require.Empty(t, errs)
	ctx := eval.NewContext(map[string]interface{}{
		"config": map[string]interface{}{"model": "gpt-5.5"},
		"sandbox": map[string]interface{}{
			"root":     "{{sandbox_root}}",
			"work_dir": "{{work_dir}}",
		},
	})
	require.NoError(t, eval.Eval(prog, ctx))
	eval.ApplyModeArgs(ctx.Result)
	eval.ApplyRemoves(ctx.Result)

	assert.Equal(t, wrapper, ctx.Result.LaunchCommand)
	assert.Equal(t, "pi-pod-lean", ctx.Result.Executable)
	assert.Equal(t, "append", ctx.Result.PromptPosition)
	assert.Equal(t, "{{sandbox_root}}/pi-home", ctx.Result.EnvVars["PI_CODING_AGENT_DIR"])
	assert.Equal(t, "{{sandbox_root}}/pi-home", ctx.Result.EnvVars["PI_POD_LEAN_BASE_DIR"])
	assert.Equal(t, "{{sandbox_root}}/pi-lean-home", ctx.Result.EnvVars["PI_POD_LEAN_PROFILE_DIR"])
	assert.Equal(t, "agentsmesh", ctx.Result.EnvVars["PI_POD_SKILLS"])
	assert.Equal(t, []string{
		"--skill", "agentsmesh",
		"--",
		"--provider", "openai-codex",
		"--model", "gpt-5.5",
		"--",
	}, ctx.Result.LaunchArgs)
}
```

- [ ] **Step 2: Run the extraction test**

Run:

```bash
bazel test //backend/internal/service/agentpod:agentpod_test --test_filter=TestExtractAgentfileOverrides_PiLeanLayerSerializesRunnableArgs
```

Expected after Task 1: PASS.

- [ ] **Step 3: Add ConfigBuilder lean command tests**

Create `backend/internal/service/agent/config_builder_pi_lean_test.go`:

```go
package agent

import (
	"context"
	"testing"

	agentDomain "github.com/anthropics/agentsmesh/backend/internal/domain/agent"
	envbundleservice "github.com/anthropics/agentsmesh/backend/internal/service/envbundle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type piLeanEnvBundleLoader struct{}

func (piLeanEnvBundleLoader) GetEffectiveForUser(_ context.Context, _, _ int64, _ string) ([]*envbundleservice.EffectiveBundle, error) {
	return nil, nil
}

type piLeanAgentProvider struct {
	source string
}

func (p piLeanAgentProvider) GetAgent(_ context.Context, slug string) (*agentDomain.Agent, error) {
	return &agentDomain.Agent{Slug: slug, AgentfileSource: &p.source}, nil
}

func TestConfigBuilder_PiLeanMergedSourceProducesWrapperCommand(t *testing.T) {
	wrapper := "/opt/agentsmesh/bin/pi-pod-lean"
	builder := NewConfigBuilder(piLeanAgentProvider{source: piLeanMergedSource(wrapper, "gpt-5.5")}, piLeanEnvBundleLoader{})

	cmd, err := builder.BuildPodCommand(context.Background(), &ConfigBuildRequest{
		AgentSlug:             "pi-cli",
		PodKey:                "pod-pi-lean-test",
		Prompt:                "build lean pi",
		MergedAgentfileSource: piLeanMergedSource(wrapper, "gpt-5.5"),
	})
	require.NoError(t, err)

	assert.Equal(t, wrapper, cmd.LaunchCommand)
	assert.Equal(t, "pty", cmd.InteractionMode)
	assert.Equal(t, "append", cmd.PromptPosition)
	assert.Equal(t, "build lean pi", cmd.Prompt)
	assert.Equal(t, "{{sandbox_root}}/pi-home", cmd.EnvVars["PI_CODING_AGENT_DIR"])
	assert.Equal(t, "{{sandbox_root}}/pi-home", cmd.EnvVars["PI_POD_LEAN_BASE_DIR"])
	assert.Equal(t, "{{sandbox_root}}/pi-lean-home", cmd.EnvVars["PI_POD_LEAN_PROFILE_DIR"])
	assert.Equal(t, "agentsmesh", cmd.EnvVars["PI_POD_SKILLS"])
	assert.Equal(t, []string{
		"--skill", "agentsmesh",
		"--",
		"--provider", "openai-codex",
		"--model", "gpt-5.5",
		"--",
	}, cmd.LaunchArgs)
}

func TestConfigBuilder_PiLeanMergedSourceOmitsEmptyModel(t *testing.T) {
	wrapper := "/opt/agentsmesh/bin/pi-pod-lean"
	builder := NewConfigBuilder(piLeanAgentProvider{source: piLeanMergedSource(wrapper, "")}, piLeanEnvBundleLoader{})

	cmd, err := builder.BuildPodCommand(context.Background(), &ConfigBuildRequest{
		AgentSlug:             "pi-cli",
		PodKey:                "pod-pi-lean-empty-model",
		Prompt:                "build lean pi",
		MergedAgentfileSource: piLeanMergedSource(wrapper, ""),
	})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"--skill", "agentsmesh",
		"--",
		"--provider", "openai-codex",
		"--",
	}, cmd.LaunchArgs)
}

func piLeanMergedSource(wrapperPath, model string) string {
	modelDefault := `"` + model + `"`
	return `AGENT "` + wrapperPath + `"
EXECUTABLE pi-pod-lean
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark", "") = ` + modelDefault + `
ENV HOME = sandbox.root + "/home"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"
ENV PI_POD_SKILLS = "agentsmesh"
PROMPT_POSITION append
arg "--skill" "agentsmesh"
arg "--"
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
`
}
```

- [ ] **Step 4: Wire the new test into Bazel**

In `backend/internal/service/agent/BUILD.bazel`, add `config_builder_pi_lean_test.go` to the `agent_test` `srcs` list.

- [ ] **Step 5: Run Task 2 tests**

Run:

```bash
bazel test //backend/internal/service/agentpod:agentpod_test --test_filter=TestExtractAgentfileOverrides_PiLeanLayerSerializesRunnableArgs
bazel test //backend/internal/service/agent:agent_test --test_filter='TestConfigBuilder_PiLeanMergedSource.*'
```

Expected: both PASS.

- [ ] **Step 6: Commit Task 2**

```bash
git add backend/internal/service/agentpod/agentfile_extract_test.go backend/internal/service/agent/config_builder_pi_lean_test.go backend/internal/service/agent/BUILD.bazel
git commit -m "test(agent): pin lean pi agentfile contract"
```

---

### Task 3: Add Pi parser aliases, dual-root scanning, and safe walk limits

**Files:**
- Modify: `runner/internal/agents/pi/register.go`
- Modify: `runner/internal/agents/pi/parser.go`
- Modify: `runner/internal/agents/pi/testsupport/fixture.go`
- Modify: `runner/internal/agents/pi/parser_test.go`
- Modify: `runner/internal/tokenusage/parser_agents_test.go`
- Modify: `runner/internal/tokenusage/parser_contract_test.go`
- Modify: `runner/internal/tokenusage/BUILD.bazel`

**Interfaces:**
- Consumes: existing Pi JSONL fixture and `tokenusage.Collect(agent, sandboxPath, startedAt)`.
- Produces: token collection succeeds for actual lean launch keys and sees both `pi-home/sessions` and `pi-lean-home/sessions` while refusing unsafe paths and enforcing visited-entry, accepted-file, depth, single-file byte, and total-byte budgets.

- [ ] **Step 1: Write parser tests for lean session roots, symlink escapes, and real scan limits**

Extend `runner/internal/agents/pi/testsupport/fixture.go` with root-specific and path-specific helpers:

```go
func BuildFixtureSandboxWithRoots(t *testing.T, roots ...string) string {
    t.Helper()
    sandbox := t.TempDir()
    for _, root := range roots {
        WriteFixtureSessionFile(t, filepath.Join(sandbox, root, "sessions", "--fixture--", "session.jsonl"))
    }
    return sandbox
}

func BuildFixtureSandbox(t *testing.T) string {
    t.Helper()
    return BuildFixtureSandboxWithRoots(t, "pi-home")
}

func WriteFixtureSessionFile(t *testing.T, path string) {
    t.Helper()
    require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
    require.NoError(t, os.WriteFile(path, fixtureSession, 0o644))
}
```

Add these concrete tests to `runner/internal/agents/pi/parser_test.go`:

- `TestPiParser_ScansLeanHomeSessions`: plant only `pi-lean-home/sessions` and assert `gpt-5.5` usage is present.
- `TestPiParser_ScansPiHomeAndLeanHomeSessions`: plant both roots and assert doubled token totals.
- `TestPiParser_RejectsPiHomeSessionFileSymlinkEscape`: plant `<outside>/session.jsonl` with valid Pi JSONL, symlink `<sandbox>/pi-home/sessions/--fixture--/escaped.jsonl` to it, and assert nil usage. This fails before the fix because current code opens the outside tokens.
- `TestPiParser_SkipsNonRegularSessionFile`: create a directory named `not-a-file.jsonl` and assert nil usage.
- `TestPiParser_EnforcesMaxAcceptedSessionFileCount`: plant `maxSessionFiles+1` valid JSONL files and assert only `maxSessionFiles` are accepted.
- `TestPiParser_EnforcesMaxSingleSessionFileBytes`: write one JSONL file with `maxSessionFileBytes+1` bytes and assert nil usage.
- `TestPiParser_EnforcesMaxTotalSessionBytes`: write enough session files to exceed `maxSessionTotalBytes` and assert the parser stops before over-budget parsing.
- `TestPiParser_EnforcesMaxVisitedWalkEntriesIncludingNonJSONL`: create `maxSessionEntries+1` non-jsonl files with lexically early names such as `000000-filler.txt`, then create the valid JSONL as `zzzz-session.jsonl`; assert the valid file is not parsed. The naming makes `filepath.WalkDir` consume the visited-entry budget before it reaches the valid JSONL file.
- `TestPiParser_EnforcesMaxRelativeDepthUnderSessions`: plant a valid file deeper than `maxSessionDepth` under `sessions` and assert nil usage.
- `TestPiParser_SkipsSymlinkedDirectoriesBeforeSuffixFiltering`: symlink a directory named `linked-dir.jsonl` to an outside fixture dir and assert nil usage.

Add imports if missing:

```go
import (
    "bytes"
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"
)
```

- [ ] **Step 2: Run parser tests and verify safety cases fail before implementation**

Run:

```bash
bazel test //runner/internal/agents/pi:pi_test --test_filter='TestPiParser_(ScansLeanHomeSessions|ScansPiHomeAndLeanHomeSessions|RejectsPiHomeSessionFileSymlinkEscape|SkipsNonRegularSessionFile|EnforcesMaxAcceptedSessionFileCount|EnforcesMaxSingleSessionFileBytes|EnforcesMaxTotalSessionBytes|EnforcesMaxVisitedWalkEntriesIncludingNonJSONL|EnforcesMaxRelativeDepthUnderSessions|SkipsSymlinkedDirectoriesBeforeSuffixFiltering)'
```

Expected before implementation: lean-root tests fail or return nil because only `pi-home/sessions` is scanned; the pi-home symlink file test fails because current code opens the outside valid JSONL file through `<sandbox>/pi-home/sessions/--fixture--/escaped.jsonl`.

- [ ] **Step 3: Implement dual-root parser scanning with limits**

In `runner/internal/agents/pi/register.go`, change parser registration:

```go
tokenusage.RegisterParser([]string{"pi", "pi-cli", "pi-pod-lean", "pi-lean-cli"}, &piParser{})
```

In `runner/internal/agents/pi/parser.go`, add constants:

```go
const (
    maxSessionFiles      = 1000
    maxSessionFileBytes  = 10 << 20
    maxSessionTotalBytes = 50 << 20
    maxSessionEntries    = 5000
    maxSessionDepth      = 8
)
```

Replace the single `sessionsDir` block in `Parse` with a loop over `[]string{"pi-home", "pi-lean-home"}` and pass a shared `piScanBudget` across roots.

Add helper types and methods below `Parse`:

```go
type piScanBudget struct {
    visitedEntries int
    acceptedFiles  int
    totalBytes     int64
    maxDepth       int
}

func (p *piParser) parseSessionsDir(sandboxPath, sessionsDir string, podStartedAt time.Time, usage *tokenusage.TokenUsage, budget *piScanBudget) error {
    if _, err := os.Stat(sessionsDir); os.IsNotExist(err) { return nil }
    if !pathContained(sandboxPath, sessionsDir) { return nil }
    return filepath.WalkDir(sessionsDir, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return nil }
        budget.visitedEntries++
        if budget.visitedEntries > maxSessionEntries { return filepath.SkipAll }
        if d.Type()&fs.ModeSymlink != 0 { return nil }
        depth, ok := relativeDepth(sessionsDir, path)
        if !ok || depth > maxSessionDepth {
            if d.IsDir() { return filepath.SkipDir }
            return nil
        }
        if depth > budget.maxDepth { budget.maxDepth = depth }
        if d.IsDir() || !strings.HasSuffix(path, ".jsonl") { return nil }
        if !pathContained(sandboxPath, path) { return nil }
        info, statErr := d.Info()
        if statErr != nil || !info.Mode().IsRegular() || info.Size() > maxSessionFileBytes { return nil }
        if budget.acceptedFiles >= maxSessionFiles || budget.totalBytes+info.Size() > maxSessionTotalBytes { return filepath.SkipAll }
        budget.acceptedFiles++
        budget.totalBytes += info.Size()
        if !tokenusage.IsModifiedAfter(path, podStartedAt) { return nil }
        if perr := parsePiSessionFile(path, usage); perr != nil { logger.Pod().Warn("Pi parser: file parse error", "file", path, "error", perr) }
        return nil
    })
}

func pathContained(root, path string) bool {
    rootReal, err := filepath.EvalSymlinks(root)
    if err != nil { return false }
    pathReal, err := filepath.EvalSymlinks(path)
    if err != nil { return false }
    rel, err := filepath.Rel(rootReal, pathReal)
    if err != nil { return false }
    return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func relativeDepth(root, path string) (int, bool) {
    rel, err := filepath.Rel(root, path)
    if err != nil { return 0, false }
    rel = filepath.Clean(rel)
    if rel == "." { return 0, true }
    if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) { return 0, false }
    return len(strings.Split(rel, string(filepath.Separator))), true
}
```

Helper contracts: `pathContained` fails closed if either path cannot be resolved and only returns true when the resolved path is inside the resolved sandbox root. `relativeDepth` counts path components relative to the `sessions` root; the root itself is depth 0, `sessions/--fixture--/session.jsonl` is depth 2, and paths outside the root return `ok=false`. The visited-entry budget increments before `.jsonl` suffix filtering. Symlink files and symlink directories are rejected before parsing; `filepath.WalkDir` does not descend symlink directories, so do not return `filepath.SkipDir` for a symlink file entry. Keep the dual-root scan for both `pi-home/sessions` and `pi-lean-home/sessions`.

- [ ] **Step 4: Add tokenusage launch-key coverage**

In `runner/internal/tokenusage/parser_agents_test.go`, import Pi for registration:

```go
_ "github.com/anthropics/agentsmesh/runner/internal/agents/pi"
```

Add cases to `TestGetParser`:

```go
{"pi", false},
{"pi-cli", false},
{"pi-pod-lean", false},
{"pi-lean-cli", false},
{"/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean", false},
```

Add `TestCollect_PiLeanActualLaunchCommand` using `BuildFixtureSandboxWithRoots(t, "pi-lean-home")` and absolute wrapper launch command. In `runner/internal/tokenusage/parser_contract_test.go`, add the same blank Pi import if not already present. In `runner/internal/tokenusage/BUILD.bazel`, add `"//runner/internal/agents/pi"` inside `tokenusage_test` deps.

- [ ] **Step 5: Run Task 3 tests**

Run:

```bash
bazel test //runner/internal/agents/pi:pi_test
bazel test //runner/internal/tokenusage:tokenusage_test --test_filter='Test(GetParser|Collect_PiLeanActualLaunchCommand|RegisteredParsers_HaveFixtureProducingNonZeroTokens|RegistryCoverage_EveryNonOptOutParserHasFixture)'
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

```bash
git add runner/internal/agents/pi/register.go runner/internal/agents/pi/parser.go runner/internal/agents/pi/testsupport/fixture.go runner/internal/agents/pi/parser_test.go runner/internal/tokenusage/parser_agents_test.go runner/internal/tokenusage/parser_contract_test.go runner/internal/tokenusage/BUILD.bazel
git commit -m "feat(pi): collect lean pod token usage safely"
```

---

### Task 4: Add non-live runner launch and agent-home containment seams

**Files:**
- Create: `runner/internal/runner/pod_builder_launch_plan.go`
- Modify: `runner/internal/runner/pod_builder_build.go`
- Create: `runner/internal/runner/pod_builder_launch_plan_test.go`
- Create: `runner/internal/runner/agent_home_containment.go`
- Modify: `runner/internal/runner/pod_builder_agent_home.go`
- Modify: `runner/internal/runner/pod_builder_agent_home_test.go`
- Modify: `runner/internal/runner/BUILD.bazel`

**Interfaces:**
- Consumes: `CreatePodCommand` from backend eval and existing `PodBuilder` setup.
- Produces: pure tests for final args/env and containment checks before any terminal starts. Agent-home target containment runs before any copy, mkdir, merge, cleanup, or log line that treats the target as trusted. Tree containment runs after copy or mkdir.

- [ ] **Step 1: Write failing launch-plan tests**

Create `runner/internal/runner/pod_builder_launch_plan_test.go` with `TestPodBuilderResolveLaunchInputs_PiLeanPromptDelimiter`. Assert the wrapper command, final args `--skill agentsmesh -- --provider openai-codex --model gpt-5.5 -- --skill should stay prompt text`, and resolved `PI_CODING_AGENT_DIR`, `PI_POD_LEAN_BASE_DIR`, and `PI_POD_LEAN_PROFILE_DIR` paths.

- [ ] **Step 2: Extract the launch-input helper**

Create `runner/internal/runner/pod_builder_launch_plan.go` with private `launchInputs` and `resolveLaunchInputs`. Move command, args, env, captured env, and prompt-position resolution out of `Build`. Keep traceparent injection after creating `launch`, and replace downstream uses with `launch.Command`, `launch.Args`, `launch.Env`, and `launch.CapturedEnv`.

- [ ] **Step 3: Write failing containment tests**

Add to `runner/internal/runner/pod_builder_agent_home_test.go`:

- `TestPrepareAgentHomeRejectsEnvPathOutsideSandboxBeforeCreation`: set `PI_CODING_AGENT_DIR` to an outside temp path, call `prepareAgentHome`, assert an error containing `agent home target escapes sandbox`, and assert the outside path was not created.
- `TestPrepareAgentHomeRejectsCopiedSymlinkEscape`: copy host `.pi/agent` containing a symlink to `/etc/passwd` and assert a containment error.
- `TestPrepareAgentHomeAllowsContainedSymlink`: copy host `.pi/agent` containing a relative symlink to `settings.json` and assert success.

Keep the copied symlink escape and contained symlink tests; add the outside env path test before creation.

- [ ] **Step 4: Implement two-stage agent-home containment validation**

Create `runner/internal/runner/agent_home_containment.go` with two validators:

- `validateAgentHomeTarget(sandboxRoot, agentHome string) error` runs before any write. It verifies sandbox root realpath, absolute target path, lexical containment under sandbox, nearest existing parent realpath under sandbox, and every existing path component used by the target resolves under sandbox.
- `validateAgentHomeContained(sandboxRoot, agentHome string) error` runs after copy or mkdir. It walks the created tree and rejects symlink escapes and paths outside sandbox.

In `runner/internal/runner/pod_builder_agent_home.go`, call target validation immediately after resolving `agentHome`, before logging, `copyDirSelective`, `os.MkdirAll`, merge work, or cleanup:

```go
agentHome = b.resolvePath(agentHome, sandboxRoot, workingDir)
if err := validateAgentHomeTarget(sandboxRoot, agentHome); err != nil {
    return err
}
```

Only call `os.RemoveAll(agentHome)` after `validateAgentHomeTarget` has passed. After copy or mkdir, walk the created tree:

```go
if err := validateAgentHomeContained(sandboxRoot, agentHome); err != nil {
    _ = os.RemoveAll(agentHome)
    return err
}
```

If this helper conflicts with Task 3's `pathContained`, keep both private to their packages. Do not create a generic utility package unless a later task needs a third caller.

- [ ] **Step 5: Wire new runner files into Bazel**

In `runner/internal/runner/BUILD.bazel`, add to `go_library.srcs`:

```python
"agent_home_containment.go",
"pod_builder_launch_plan.go",
```

Add to `runner_test.srcs`:

```python
"pod_builder_launch_plan_test.go",
```

- [ ] **Step 6: Run Task 4 tests**

Run:

```bash
bazel test //runner/internal/runner:runner_test --test_filter='TestPodBuilderResolveLaunchInputs_PiLeanPromptDelimiter|TestPrepareAgentHome(RejectsEnvPathOutsideSandboxBeforeCreation|RejectsCopiedSymlinkEscape|AllowsContainedSymlink)'
```

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

```bash
git add runner/internal/runner/pod_builder_launch_plan.go runner/internal/runner/pod_builder_build.go runner/internal/runner/pod_builder_launch_plan_test.go runner/internal/runner/agent_home_containment.go runner/internal/runner/pod_builder_agent_home.go runner/internal/runner/pod_builder_agent_home_test.go runner/internal/runner/BUILD.bazel
git commit -m "feat(runner): verify lean pi launch inputs safely"
```

---

### Task 5: Add non-live preflight and redacted daemon inspection helpers

**Files:**
- Create: `tools/pi-lean/preflight.sh`
- Create: `tools/pi-lean/inspect-pod-daemon-redacted.sh`
- Create: `tools/pi-lean/BUILD.bazel`

**Interfaces:**
- Consumes: `/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` and a local temp sandbox.
- Produces: local-only wrapper preflight and safe daemon-state inspection tools. These helpers do not create pods and do not call the AgentsMesh API.
- Validated facts: wrapper realpath, owner, group, mode, hash, dry-run command shape, temp sandbox containment, and any resolved `--skill <path>` values.
- Operator-reviewed facts: approved wrapper install path, expected owner/group/mode values when local policy requires exact values, and `EXPECTED_WRAPPER_SHA256`. If the workstation wrapper or parent dirs are still mode `0770`, fix the permissions or set an explicit operator-approved override for that one run; do not silently pass group-writable paths.

- [ ] **Step 1: Create the non-live wrapper preflight script**

Create `tools/pi-lean/preflight.sh`. It must support `--syntax-only` by exiting 0 before touching the filesystem, and normal mode must:

1. Create `PROFILE_DIR` before containment checks, or use `realpath -m` only for paths not yet created.
2. Compute wrapper realpath and require it to equal `APPROVED_WRAPPER_PATH`, defaulting to `/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean`.
3. Reject wrapper symlink surprises unless the realpath equals the approved path.
4. Check owner with `stat`, defaulting to `$(id -un)` unless `EXPECTED_WRAPPER_OWNER` is set.
5. Check group with `stat`, defaulting to `$(id -gn)` unless `EXPECTED_WRAPPER_GROUP` is set.
6. Check mode with `stat`; default rejects group/world writable wrapper and parent dirs. Check at least the wrapper's immediate parent and approved install root. If mode is still `0770`, fix permissions or use an explicit operator-approved override.
7. Check `EXPECTED_WRAPPER_SHA256`. If unset, print the observed hash and fail with instructions to rerun with that expected hash or source it from a local env file.
8. Run the wrapper dry-run in a temp sandbox and verify command shape without creating a pod.
9. Parse dry-run output and verify every `--skill <path>` value resolves under the temp sandbox profile/base/home roots.
10. Verify generated profile symlinks resolve under the temp sandbox.

Use this skill-path containment helper inside the shell script:

```bash
verify_skill_paths() {
    local output="$1"
    DRY_RUN_OUTPUT="${output}" python3 - "${BASE_DIR}" "${PROFILE_DIR}" "${HOME_DIR}" <<'PY'
import os
import shlex
import sys
from pathlib import Path

roots = [Path(arg).resolve() for arg in sys.argv[1:]]
args = shlex.split(os.environ["DRY_RUN_OUTPUT"])
for index, value in enumerate(args[:-1]):
    if value != "--skill":
        continue
    candidate = args[index + 1]
    if "/" not in candidate and not candidate.endswith(".md"):
        continue
    real = Path(candidate).resolve()
    if not any(real == root or root in real.parents for root in roots):
        raise SystemExit(f"skill path escapes temp sandbox: {candidate} -> {real}")
PY
}
```

- [ ] **Step 2: Create the redacted daemon-state inspection script**

Create `tools/pi-lean/inspect-pod-daemon-redacted.sh`. It must support `--syntax-only` by exiting 0 before touching the filesystem. The helper is safe for command-shape inspection, not prompt-content inspection.

Update the Python redaction snippet so `args` is summarized:

```python
SECRET_MARKERS = ("token", "key", "auth", "password", "secret")
STRUCTURAL_VALUE_FLAGS = {"--skill", "--provider", "--model"}

def has_secret_marker(value: str) -> bool:
    lowered = value.lower()
    return any(marker in lowered for marker in SECRET_MARKERS)

def is_secret_assignment(value: str) -> bool:
    if "=" not in value:
        return False
    key, _ = value.split("=", 1)
    return has_secret_marker(key)

def redact_assignment(value: str) -> str:
    key, _ = value.split("=", 1)
    return f"{key}=<redacted>"

def is_secret_flag(value: str) -> bool:
    return value.startswith("--") and has_secret_marker(value.split("=", 1)[0])

def redact_args(items):
    args = [str(item) for item in (items or [])]
    delimiter_positions = [index for index, value in enumerate(args) if value == "--"]
    final_delimiter = delimiter_positions[-1] if delimiter_positions else -1
    limit = final_delimiter + 1 if final_delimiter >= 0 else len(args)
    safe = []
    index = 0
    while index < limit:
        value = args[index]
        if is_secret_assignment(value):
            safe.append(redact_assignment(value))
            index += 1
            continue
        if is_secret_flag(value):
            safe.append(value)
            if index + 1 < limit and "=" not in value:
                safe.append("<redacted>")
                index += 2
                continue
            index += 1
            continue
        safe.append(value)
        if value in STRUCTURAL_VALUE_FLAGS and index + 1 < limit:
            safe.append(args[index + 1])
            index += 2
            continue
        index += 1
    if final_delimiter >= 0 and final_delimiter < len(args) - 1:
        safe.append("<prompt redacted>")
    return safe
```

Redaction must happen before appending a raw arg. The helper must cover split forms such as `--api-key secret`, inline forms such as `--api-key=secret`, and environment-style arg strings such as `OPENAI_API_KEY=secret`.

Use `"args": redact_args(state.get("args", []))` in `safe_state`. Keep the existing env redaction.

- [ ] **Step 3: Add concrete Bazel shell-test wiring**

Create `tools/pi-lean/BUILD.bazel`:

```python
sh_test(
    name = "preflight_syntax_test",
    srcs = ["preflight.sh"],
    args = ["--syntax-only"],
)

sh_test(
    name = "inspect_pod_daemon_redacted_syntax_test",
    srcs = ["inspect-pod-daemon-redacted.sh"],
    args = ["--syntax-only"],
)
```

Both shell scripts must support `--syntax-only` by exiting 0 after Bash parses function definitions and before touching the filesystem.

- [ ] **Step 4: Make both scripts executable**

Run:

```bash
chmod +x tools/pi-lean/preflight.sh tools/pi-lean/inspect-pod-daemon-redacted.sh
```

Expected: command exits 0.

- [ ] **Step 5: Run non-live script checks**

Run:

```bash
bash -n tools/pi-lean/preflight.sh
bash -n tools/pi-lean/inspect-pod-daemon-redacted.sh
bazel test //tools/pi-lean:preflight_syntax_test //tools/pi-lean:inspect_pod_daemon_redacted_syntax_test
EXPECTED_WRAPPER_SHA256=<hash> EXPECTED_WRAPPER_OWNER="$(id -un)" EXPECTED_WRAPPER_GROUP="$(id -gn)" tools/pi-lean/preflight.sh /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean
```

Set `EXPECTED_WRAPPER_MODE=<mode>` only when an exact wrapper file mode is part of the local approval. If the observed wrapper or checked parent dirs are group/world writable, fix permissions first or rerun with `ALLOW_GROUP_WRITABLE_WRAPPER=operator-approved-0770` after operator approval.

Expected:

```text
preflight passed
```

The command may print timestamped info lines before that final line. It must not create a pod, call AgentsMesh APIs, write DB state, rebuild images, or deploy.

- [ ] **Step 6: Add a local fixture check for redaction**

Run:

```bash
tmpdir="$(mktemp -d)"
cat > "${tmpdir}/pod_daemon.json" <<'JSON'
{
  "pod_key": "pi-lean-check",
  "agent": "/opt/agentsmesh/bin/pi-pod-lean",
  "sandbox_path": "/tmp/pi-lean-check",
  "work_dir": "/tmp/pi-lean-check/work",
  "command": "/opt/agentsmesh/bin/pi-pod-lean",
  "args": ["--skill", "agentsmesh", "--", "--provider", "openai-codex", "--api-key", "secret", "--auth-token=secret", "OPENAI_API_KEY=secret", "--", "hello prompt"],
  "env": ["PI_CODING_AGENT_DIR=/tmp/pi-lean-check/pi-home", "OPENAI_API_KEY=secret", "AUTH_TOKEN=secret", "PI_POD_SKILLS=agentsmesh"],
  "perpetual": false,
  "started_at": "2026-06-24T00:00:00Z"
}
JSON
tools/pi-lean/inspect-pod-daemon-redacted.sh "${tmpdir}" | tee "${tmpdir}/redacted.json"
grep -q '<redacted>' "${tmpdir}/redacted.json"
grep -q '<prompt redacted>' "${tmpdir}/redacted.json"
! grep -q 'secret' "${tmpdir}/redacted.json"
! grep -q 'hello prompt' "${tmpdir}/redacted.json"
python3 -c 'import shutil, sys; shutil.rmtree(sys.argv[1])' "${tmpdir}"
```

Expected: command exits 0.

- [ ] **Step 7: Commit Task 5**

```bash
git add tools/pi-lean/preflight.sh tools/pi-lean/inspect-pod-daemon-redacted.sh tools/pi-lean/BUILD.bazel
git commit -m "chore(pi): add lean pod validation helpers"
```

---

### Task 6: Final non-live verification and execution handoff

**Files:**
- Modify only if Task 1 to Task 5 surfaced required doc updates: `docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md`
- Read-only verification: all files touched by Tasks 1 to 5

**Interfaces:**
- Consumes: completed Task 1 to Task 5 commits.
- Produces: a green, non-live implementation branch ready for adversarial code review, but still not authorized for live pod creation.

**Base-ref precondition:** After the Rev 2 plan commit lands and before Task 1 implementation starts, capture the implementation base:

```bash
export IMPLEMENTATION_BASE_SHA="$(git rev-parse HEAD)"
```

Record that value in the implementation handoff. Final scans use this explicit base ref so they do not scan unrelated pre-existing tree content.

- [ ] **Step 1: Run focused tests**

Run:

```bash
bazel test //agentfile/merge:merge_test //agentfile/serialize:serialize_test
bazel test //backend/internal/service/agentpod:agentpod_test --test_filter=TestExtractAgentfileOverrides_PiLeanLayerSerializesRunnableArgs
bazel test //backend/internal/service/agent:agent_test --test_filter='TestConfigBuilder_PiLeanMergedSource.*'
bazel test //runner/internal/agents/pi:pi_test
bazel test //runner/internal/tokenusage:tokenusage_test --test_filter='Test(GetParser|Collect_PiLeanActualLaunchCommand|RegisteredParsers_HaveFixtureProducingNonZeroTokens|RegistryCoverage_EveryNonOptOutParserHasFixture)'
bazel test //runner/internal/runner:runner_test --test_filter='TestPodBuilderResolveLaunchInputs_PiLeanPromptDelimiter|TestPrepareAgentHome(RejectsEnvPathOutsideSandboxBeforeCreation|RejectsCopiedSymlinkEscape|AllowsContainedSymlink)'
bazel test //tools/pi-lean:preflight_syntax_test //tools/pi-lean:inspect_pod_daemon_redacted_syntax_test
```

Expected: all PASS.

- [ ] **Step 2: Run package-level tests for modified areas**

Run:

```bash
bazel test //agentfile/...
bazel test //backend/internal/service/agent:agent_test //backend/internal/service/agentpod:agentpod_test
bazel test //runner/internal/agents/pi:pi_test //runner/internal/tokenusage:tokenusage_test //runner/internal/runner:runner_test
bazel test //tools/pi-lean:preflight_syntax_test //tools/pi-lean:inspect_pod_daemon_redacted_syntax_test
```

Expected: all PASS. If `//runner/internal/runner:runner_test` exposes unrelated pre-existing flakes, capture the exact failing test and run the focused Task 4 filters again before escalating.

- [ ] **Step 3: Run shell validations**

Run:

```bash
bash -n tools/pi-lean/preflight.sh
bash -n tools/pi-lean/inspect-pod-daemon-redacted.sh
EXPECTED_WRAPPER_SHA256=<hash> EXPECTED_WRAPPER_OWNER="$(id -un)" EXPECTED_WRAPPER_GROUP="$(id -gn)" tools/pi-lean/preflight.sh /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean
```

Expected: all commands exit 0; preflight prints `preflight passed`.

- [ ] **Step 4: Scan forbidden placeholders and long dashes only in changed or plan-owned files**

Run:

```bash
mapfile -d '' -t changed_files < <(git diff -z --name-only "${IMPLEMENTATION_BASE_SHA}..HEAD" -- agentfile backend runner tools/pi-lean docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md)
scan_files_tmp="$(mktemp)"
trap 'rm -f "${scan_files_tmp}"' EXIT
printf '%s\0' 'docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md' "${changed_files[@]}" | sort -zu > "${scan_files_tmp}"

placeholder_found=0
while IFS= read -r -d '' file; do
    [[ -f "${file}" ]] || continue
    if grep -nE 'T[B]D|T[O]DO|implement[[:space:]]+later|fill[[:space:]]+in[[:space:]]+details' -- "${file}"; then
        placeholder_found=1
    else
        status=$?
        [[ ${status} -eq 1 ]] || exit "${status}"
    fi
done < "${scan_files_tmp}"
[[ ${placeholder_found} -eq 0 ]] || exit 1

long_dash_found=0
while IFS= read -r -d '' file; do
    [[ -f "${file}" ]] || continue
    if grep -nP '[\x{2013}\x{2014}]' -- "${file}"; then
        long_dash_found=1
    else
        status=$?
        [[ ${status} -eq 1 ]] || exit "${status}"
    fi
done < "${scan_files_tmp}"
[[ ${long_dash_found} -eq 0 ]] || exit 1
```

Expected: no output and exit 0. This path-safe form avoids `xargs` status ambiguity: grep exit 1 means no matches, grep exit greater than 1 is a real error and fails the step, and any match sets a flag that fails after all files are reported. This replaces tree-wide scans over `agentfile backend runner`. The scan target set is the plan file plus files changed by the implementation branch relative to `IMPLEMENTATION_BASE_SHA`.

- [ ] **Step 5: Record working-tree state and process attestation**

Run:

```bash
git status --short
```

Expected: only intentional code, tests, docs, and tool files are modified or committed. `git status` proves repository state only. Absence of pod creation, DB writes, backend/web rebuilds, and deploys is enforced by command discipline plus operator/process attestation unless a future revision adds a specific runtime log check.

- [ ] **Step 6: Commit Task 6 only if docs changed**

If Task 6 changed docs, commit only those docs:

```bash
git add docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md
git commit -m "docs(pi): update lean pod implementation notes"
```

If no docs changed, do not create an empty commit.

---

## Gated live validation appendix

Do not execute this appendix during plan implementation. It is written here only so the later operator authorization is precise.

**Do NOT create a live lean Pi pod before the operator types `fire create-pi-lean-pod`.**

After that exact token, the authorized scope is one laptop-runner pod only:

1. Run `tools/pi-lean/preflight.sh /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` with `EXPECTED_WRAPPER_SHA256=<hash>` and any approved owner/group/mode variables.
2. Resolve the laptop runner ID explicitly from the safe backend/UI path chosen by the operator. Do not auto-select a runner.
3. Create exactly one non-perpetual pod with fixed alias `pi-lean-validation-2026-06-24`, `agent_slug=pi-cli`, explicit `runner_id=<laptop runner id>`, and the canonical lean AgentFile layer.
4. Inspect daemon state only through `tools/pi-lean/inspect-pod-daemon-redacted.sh <sandbox-path>`.
5. Verify command, args, pre-exec env, workdir, `<sandbox>/pi-home`, and `<sandbox>/pi-lean-home/sessions`.
6. Verify token usage collection through the actual launch key.
7. Before sandbox cleanup, verify all cleanup guards:
   - Redacted daemon metadata `pod_key` equals `pi-lean-validation-2026-06-24`.
   - Sandbox path is non-empty.
   - Sandbox realpath is under the laptop runner workspace root chosen by the operator.
   - Sandbox path belongs to the fixed pod key.
   - Token usage collection has been verified.
   - Cleanup command uses no globs.
8. Terminate the pod and remove only that pod's sandbox after token usage is collected and the cleanup guards pass.
9. If any check fails, terminate by pod key and inspect only redacted logs.

Hetzner validation, production deployment, backend image rebuild, and web image rebuild remain out of scope.

---

## Rollback

Prototype rollback:

- Stop sending the lean AgentFile layer.
- Use existing `pi-cli`.
- Remove only temp dirs from `tools/pi-lean/preflight.sh` or the single gated validation sandbox.
- Leave parser aliases in place; unused aliases are inert and help preserve token collection for any pod already launched with the wrapper path.

Code rollback:

- Revert the task commit that introduced the issue.
- If reverting Task 3 after a live validation ever happened, do not remove parser aliases until all lean pods have terminated and token usage has been collected.
- Do not alter `backend/migrations/000160_add_pi_agent.up.sql` as part of this plan.

---

## Resolutions Applied in Rev 3

- R2-COR-01 applied: Task 3 now defines parser helper contracts and snippets for `pathContained` and `relativeDepth`, removes the undefined `symlinkPointsToDir` call, and clarifies symlink handling without unsafe `SkipDir` use on symlink file entries.
- R2-TEST-01 applied: Task 6 now uses a path-safe `git diff -z` and loop-based grep scan that distinguishes no-match from real grep errors instead of masking failures through `xargs` fallback semantics.
- R2-SEC-01 applied: Task 5 now redacts split secret flags, inline secret assignments such as `--api-key=secret`, and environment-style arg strings before appending raw argv values. The fixture covers split, inline, and environment-style secret forms.
- Cross-model review applied: headless Sonnet reviewed the Rev 3 diff and returned PASS with no blockers. Non-blocking notes did not require plan edits.

---

## Resolutions Applied in Rev 2

- C1 applied: Task 3 now covers dual session roots, pi-home symlink escape rejection, non-regular files, symlink directories, visited-entry budget, max depth, max accepted files, max single-file bytes, and max total bytes. Parser snippets increment visited entries before suffix filtering and reject symlinks before parsing.
- C2 applied: Task 4 now uses two-stage agent-home validation. `validateAgentHomeTarget` runs before writes or cleanup, and `validateAgentHomeContained` walks the created tree after copy or mkdir.
- C3 applied: Task 5 separates validated facts from operator-reviewed facts and requires wrapper realpath, owner, group, mode, parent-dir mode, hash, dry-run shape, and resolved skill path containment checks.
- C4 applied: Task 5 redacts daemon argv after the final prompt delimiter and redacts values following secret-looking flags. The helper is scoped to command-shape inspection.
- C5 applied: Task 5 keeps `tools/pi-lean/BUILD.bazel` and defines concrete `sh_test` syntax targets. Task 5 and Task 6 run those targets.
- C6 applied: Task 6 captures `IMPLEMENTATION_BASE_SHA` and scans only the plan plus files changed by the implementation branch under the explicit path set.
- C7 applied: Task 6 treats `git status --short` as repository-state evidence only and records live-operation absence through command discipline plus operator/process attestation.
- C8 applied: the gated live appendix now requires fixed pod-key, non-empty sandbox, runner-root containment, fixed-pod path ownership, no-glob cleanup, and verified token usage before sandbox removal.
- C9 applied: Task 1 documents `REMOVE arg` as first-literal merge-time base-statement replacement, limited here to `--provider` and `--model`.
- Deferred: F-TEST-06 and F-SEC-08 remain review-context artifacts about missing `/home/malfirg/programming_projects/AgentsMesh/plan.md` and `progress.md`, not defects in this plan. No plan change.
- Deferred: cross-model review is outside this edit. Status remains awaiting cross-model review.

---

## Self-review

### Spec coverage

- Separate lean path without mutating `pi-cli`: covered by Task 2 and no migration task.
- AgentFile `AGENT` command override: covered by Task 2.
- `REMOVE arg` feasibility: covered by Task 1, which fixes the current post-eval removal hazard.
- Wrapper delimiter and prompt safety: covered by Task 2 and Task 4.
- Pre-exec versus runtime `PI_CODING_AGENT_DIR`: covered by Task 2, Task 4, and Task 5.
- Named skill opt-in: covered by Task 2 and Task 5.
- Wrapper preflight: covered by Task 5.
- Token aliases and dual session roots: covered by Task 3.
- Parser containment and scan limits: covered by Task 3.
- Copied agent home containment: covered by Task 4.
- Non-live runner seam: covered by Task 4.
- Redacted live inspection tooling: covered by Task 5.
- Live validation gate: covered by the gated appendix.

### Placeholder scan

This plan intentionally contains no deferred-work placeholders and no unspecified code steps. Any later revision must keep that property.

### Type consistency

- `launchInputs` is private to `runner/internal/runner` and consumed only by `PodBuilder` tests and `Build`.
- `BuildFixtureSandboxWithRoots` returns the same sandbox string shape as existing `BuildFixtureSandbox`.
- AgentFile expected args are identical across Task 1, Task 2, and Task 4.
- The live validation appendix does not authorize itself; it remains gated by the exact verbal token.
