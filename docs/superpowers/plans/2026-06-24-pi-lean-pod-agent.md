# Lean Pi Pod Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** Rev 1 - awaiting adversarial and cross-model review.

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
  - Add tests for lean-home sessions, dual roots, symlink escape rejection, non-regular file rejection, and scan limits.
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
  - Validate copied agent home paths after copy or mkdir.
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
  - Add `sh_test` targets for `bash -n` and fixture checks if repository shell rules are available; otherwise document direct `bash -n` commands in the plan verification step.

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
- Produces: token collection succeeds for actual lean launch keys and sees both `pi-home/sessions` and `pi-lean-home/sessions` without following unsafe paths.

- [ ] **Step 1: Write parser tests for lean session roots and unsafe paths**

Extend `runner/internal/agents/pi/testsupport/fixture.go` with a root-specific helper:

```go
func BuildFixtureSandboxWithRoots(t *testing.T, roots ...string) string {
	t.Helper()
	sandbox := t.TempDir()
	for _, root := range roots {
		dir := filepath.Join(sandbox, root, "sessions", "--fixture--")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("pi fixture: mkdir %s: %v", root, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), fixtureSession, 0o644); err != nil {
			t.Fatalf("pi fixture: write %s: %v", root, err)
		}
	}
	return sandbox
}
```

Change existing `BuildFixtureSandbox` to call it:

```go
func BuildFixtureSandbox(t *testing.T) string {
	t.Helper()
	return BuildFixtureSandboxWithRoots(t, "pi-home")
}
```

Add tests to `runner/internal/agents/pi/parser_test.go`:

```go
func TestPiParser_ScansLeanHomeSessions(t *testing.T) {
	sandbox := testsupport.BuildFixtureSandboxWithRoots(t, "pi-lean-home")

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.NotNil(t, usage.Models["gpt-5.5"])
}

func TestPiParser_ScansPiHomeAndLeanHomeSessions(t *testing.T) {
	sandbox := testsupport.BuildFixtureSandboxWithRoots(t, "pi-home", "pi-lean-home")

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	require.NotNil(t, usage)

	m := usage.Models["gpt-5.5"]
	require.NotNil(t, m)
	assert.Equal(t, int64(2*(17341+2048)), m.InputTokens)
	assert.Equal(t, int64(2*(177+512)), m.OutputTokens)
}

func TestPiParser_RejectsSessionSymlinkEscape(t *testing.T) {
	sandbox := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "sessions"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(sandbox, "pi-lean-home"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "sessions"), filepath.Join(sandbox, "pi-lean-home", "sessions")))

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	assert.Nil(t, usage)
}

func TestPiParser_SkipsNonRegularSessionFile(t *testing.T) {
	sandbox := t.TempDir()
	dir := filepath.Join(sandbox, "pi-lean-home", "sessions", "--fixture--")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "not-a-file.jsonl"), 0o755))

	usage, err := (&piParser{}).Parse(sandbox, time.Unix(0, 0))
	require.NoError(t, err)
	assert.Nil(t, usage)
}
```

Add imports if missing:

```go
import (
	"os"
	"path/filepath"
	"testing"
	"time"
)
```

- [ ] **Step 2: Run parser tests and verify lean-root tests fail before implementation**

Run:

```bash
bazel test //runner/internal/agents/pi:pi_test --test_filter='TestPiParser_(ScansLeanHomeSessions|ScansPiHomeAndLeanHomeSessions|RejectsSessionSymlinkEscape|SkipsNonRegularSessionFile)'
```

Expected before implementation: lean-root tests FAIL or return nil because only `pi-home/sessions` is scanned.

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
)
```

Replace the single `sessionsDir` block in `Parse` with:

```go
roots := []string{"pi-home", "pi-lean-home"}
var scanned piScanBudget
for _, root := range roots {
	sessionsDir := filepath.Join(sandboxPath, root, "sessions")
	if err := p.parseSessionsDir(sandboxPath, sessionsDir, podStartedAt, usage, &scanned); err != nil {
		logger.Pod().Warn("Pi parser: walk error", "dir", sessionsDir, "error", err)
	}
}
```

Add helper types and methods below `Parse`:

```go
type piScanBudget struct {
	files int
	bytes int64
}

func (p *piParser) parseSessionsDir(sandboxPath, sessionsDir string, podStartedAt time.Time, usage *tokenusage.TokenUsage, budget *piScanBudget) error {
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return nil
	}
	if !pathContained(sandboxPath, sessionsDir) {
		return nil
	}

	return filepath.WalkDir(sessionsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !pathContained(sandboxPath, path) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > maxSessionFileBytes {
			return nil
		}
		if budget.files >= maxSessionFiles || budget.bytes+info.Size() > maxSessionTotalBytes {
			return filepath.SkipAll
		}
		budget.files++
		budget.bytes += info.Size()
		if !tokenusage.IsModifiedAfter(path, podStartedAt) {
			return nil
		}
		if perr := parsePiSessionFile(path, usage); perr != nil {
			logger.Pod().Warn("Pi parser: file parse error", "file", path, "error", perr)
		}
		return nil
	})
}

func pathContained(root, path string) bool {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	pathReal, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootReal, pathReal)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
```

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

Add a collect test:

```go
func TestCollect_PiLeanActualLaunchCommand(t *testing.T) {
	sandbox := pifixture.BuildFixtureSandboxWithRoots(t, "pi-lean-home")
	usage := tokenusage.Collect("/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean", sandbox, epoch)
	require.NotNil(t, usage)
	assert.NotNil(t, usage.Models["gpt-5.5"])
}
```

Update imports in `parser_agents_test.go`:

```go
pifixture "github.com/anthropics/agentsmesh/runner/internal/agents/pi/testsupport"
```

In `runner/internal/tokenusage/parser_contract_test.go`, add the same blank Pi import if not already present in that package's test files after Step 4. This keeps registry coverage deterministic if tests are filtered by file or package setup changes later.

In `runner/internal/tokenusage/BUILD.bazel`, add:

```python
"//runner/internal/agents/pi",
```

inside `tokenusage_test` deps.

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
- Produces: pure tests for final args/env and containment checks before any terminal starts.

- [ ] **Step 1: Write failing launch-plan tests**

Create `runner/internal/runner/pod_builder_launch_plan_test.go`:

```go
package runner

import (
	"testing"

	runnerv1 "github.com/anthropics/agentsmesh/proto/gen/go/runner/v1"
	"github.com/anthropics/agentsmesh/runner/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodBuilderResolveLaunchInputs_PiLeanPromptDelimiter(t *testing.T) {
	builder := NewPodBuilder(PodBuilderDeps{Config: &config.Config{WorkspaceRoot: t.TempDir()}}).WithCommand(&runnerv1.CreatePodCommand{
		PodKey:        "pi-lean-launch",
		LaunchCommand: "/opt/agentsmesh/bin/pi-pod-lean",
		LaunchArgs: []string{
			"--skill", "agentsmesh",
			"--",
			"--provider", "openai-codex",
			"--model", "gpt-5.5",
			"--",
		},
		Prompt:         "--skill should stay prompt text",
		PromptPosition: "append",
		EnvVars: map[string]string{
			"PI_CODING_AGENT_DIR":       "{{sandbox_root}}/pi-home",
			"PI_POD_LEAN_BASE_DIR":      "{{sandbox_root}}/pi-home",
			"PI_POD_LEAN_PROFILE_DIR":   "{{sandbox_root}}/pi-lean-home",
			"PI_POD_SKILLS":             "agentsmesh",
		},
	})

	plan, err := builder.resolveLaunchInputs("/tmp/sandbox", "/tmp/sandbox/work")
	require.NoError(t, err)
	assert.Equal(t, "/opt/agentsmesh/bin/pi-pod-lean", plan.Command)
	assert.Equal(t, []string{
		"--skill", "agentsmesh",
		"--",
		"--provider", "openai-codex",
		"--model", "gpt-5.5",
		"--",
		"--skill should stay prompt text",
	}, plan.Args)
	assert.Equal(t, "/tmp/sandbox/pi-home", plan.Env["PI_CODING_AGENT_DIR"])
	assert.Equal(t, "/tmp/sandbox/pi-home", plan.Env["PI_POD_LEAN_BASE_DIR"])
	assert.Equal(t, "/tmp/sandbox/pi-lean-home", plan.Env["PI_POD_LEAN_PROFILE_DIR"])
}
```

This test should fail before implementation because `resolveLaunchInputs` does not exist.

- [ ] **Step 2: Extract the launch-input helper**

Create `runner/internal/runner/pod_builder_launch_plan.go`:

```go
package runner

import "fmt"

type launchInputs struct {
	Command     string
	Args        []string
	Env         map[string]string
	CapturedEnv []string
}

func (b *PodBuilder) resolveLaunchInputs(sandboxRoot, workingDir string) (*launchInputs, error) {
	if b.cmd == nil {
		return nil, fmt.Errorf("command is required")
	}
	resolvedArgs := resolveStringSlice(b.cmd.LaunchArgs, sandboxRoot, workingDir)
	envVars := b.mergeEnvVars(sandboxRoot)
	for k, v := range b.cmd.EnvVars {
		envVars[k] = resolvePathPlaceholders(v, sandboxRoot, workingDir)
	}

	if prompt := b.cmd.Prompt; prompt != "" {
		switch b.cmd.PromptPosition {
		case "prepend":
			resolvedArgs = append([]string{prompt}, resolvedArgs...)
		case "append":
			resolvedArgs = append(resolvedArgs, prompt)
		}
	}

	return &launchInputs{
		Command:     b.cmd.LaunchCommand,
		Args:        resolvedArgs,
		Env:         envVars,
		CapturedEnv: buildMergedEnv(envVars),
	}, nil
}
```

Modify `runner/internal/runner/pod_builder_build.go` after setup:

```go
launch, err := b.resolveLaunchInputs(sandboxRoot, workingDir)
if err != nil {
	return nil, err
}
if err := b.createFilesFromProto(b.cmd.FilesToCreate, sandboxRoot, workingDir); err != nil {
	return nil, err
}
```

Then replace downstream `resolvedArgs`, `envVars`, `capturedEnv`, and `launchCommand` uses with `launch.Args`, `launch.Env`, `launch.CapturedEnv`, and `launch.Command`. Keep traceparent injection after creating `launch`:

```go
injectTraceparent(ctx, launch.Env)
if tp, ok := launch.Env["TRACEPARENT"]; ok {
	launch.CapturedEnv = append(launch.CapturedEnv, "TRACEPARENT="+tp)
}
```

- [ ] **Step 3: Write failing containment tests**

Add to `runner/internal/runner/pod_builder_agent_home_test.go`:

```go
func TestPrepareAgentHomeRejectsCopiedSymlinkEscape(t *testing.T) {
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	t.Setenv("USERPROFILE", hostHome)

	source := filepath.Join(hostHome, ".pi", "agent")
	require.NoError(t, os.MkdirAll(source, 0o755))
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(source, "escaped")))

	sandboxRoot := t.TempDir()
	builder := &PodBuilder{cmd: &runnerv1.CreatePodCommand{
		PodKey: "pi-home-escape",
		EnvVars: map[string]string{
			"PI_CODING_AGENT_DIR": filepath.Join(sandboxRoot, "pi-home"),
		},
	}}

	err := builder.prepareAgentHome(sandboxRoot, filepath.Join(sandboxRoot, "work"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent home escapes sandbox")
}

func TestPrepareAgentHomeAllowsContainedSymlink(t *testing.T) {
	hostHome := t.TempDir()
	t.Setenv("HOME", hostHome)
	t.Setenv("USERPROFILE", hostHome)

	source := filepath.Join(hostHome, ".pi", "agent")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "skills", "agentsmesh"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "settings.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.Symlink("settings.json", filepath.Join(source, "settings-link.json")))

	sandboxRoot := t.TempDir()
	builder := &PodBuilder{cmd: &runnerv1.CreatePodCommand{
		PodKey: "pi-home-contained",
		EnvVars: map[string]string{
			"PI_CODING_AGENT_DIR": filepath.Join(sandboxRoot, "pi-home"),
		},
	}}

	err := builder.prepareAgentHome(sandboxRoot, filepath.Join(sandboxRoot, "work"))
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(sandboxRoot, "pi-home", "settings.json"))
}
```

- [ ] **Step 4: Implement agent-home containment validation**

Create `runner/internal/runner/agent_home_containment.go`:

```go
package runner

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

func validateAgentHomeContained(sandboxRoot, agentHome string) error {
	if !pathInside(sandboxRoot, agentHome) {
		return fmt.Errorf("agent home escapes sandbox: %s", agentHome)
	}
	return filepath.WalkDir(agentHome, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !pathInside(sandboxRoot, path) {
			return fmt.Errorf("agent home escapes sandbox: %s", path)
		}
		if d.Type()&fs.ModeSymlink != 0 && !pathInside(sandboxRoot, path) {
			return fmt.Errorf("agent home symlink escapes sandbox: %s", path)
		}
		return nil
	})
}

func pathInside(root, path string) bool {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	pathReal, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootReal, pathReal)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
```

In `runner/internal/runner/pod_builder_agent_home.go`, call the validator before returning:

```go
if err := validateAgentHomeContained(sandboxRoot, agentHome); err != nil {
	_ = os.RemoveAll(agentHome)
	return err
}

if spec.MergeConfig == nil {
	return nil
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
bazel test //runner/internal/runner:runner_test --test_filter='TestPodBuilderResolveLaunchInputs_PiLeanPromptDelimiter|TestPrepareAgentHome(RejectsCopiedSymlinkEscape|AllowsContainedSymlink)'
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
- Create: `tools/pi-lean/BUILD.bazel` if a local `sh_test` pattern is available; otherwise use direct `bash -n` checks.

**Interfaces:**
- Consumes: `/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean` and a local temp sandbox.
- Produces: local-only wrapper preflight and safe daemon-state inspection tools. These helpers do not create pods and do not call the AgentsMesh API.

- [ ] **Step 1: Create the non-live wrapper preflight script**

Create `tools/pi-lean/preflight.sh`:

```bash
#!/bin/bash
set -euo pipefail

log_info()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO:  $*"; }
log_warn()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN:  $*" >&2; }
log_error() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2; }

cleanup() {
    local exit_code=$?
    [[ ${exit_code} -ne 0 ]] && log_error "Failed with exit code ${exit_code}"
    [[ -n "${TMP_DIR:-}" && -d "${TMP_DIR}" ]] && rm -rf "${TMP_DIR}"
}
trap 'cleanup' EXIT

readonly DEFAULT_WRAPPER="/home/malfirg/programming_projects/pi-config/bin/pi-pod-lean"
readonly WRAPPER_PATH="${1:-${DEFAULT_WRAPPER}}"
readonly EXPECTED_SKILL="agentsmesh"
TMP_DIR="$(mktemp -d)"
readonly TMP_DIR
readonly BASE_DIR="${TMP_DIR}/pi-home"
readonly PROFILE_DIR="${TMP_DIR}/pi-lean-home"
readonly HOME_DIR="${TMP_DIR}/home"

require_file() {
    local path="$1"
    [[ -f "${path}" ]] || { log_error "missing file: ${path}"; exit 1; }
}

require_contained() {
    local path="$1"
    local real
    real="$(realpath "${path}")"
    case "${real}" in
        "${TMP_DIR}"|"${TMP_DIR}"/*) return 0 ;;
        *) log_error "path escapes temp sandbox: ${real}"; exit 1 ;;
    esac
}

main() {
    require_file "${WRAPPER_PATH}"
    [[ -x "${WRAPPER_PATH}" ]] || { log_error "wrapper not executable: ${WRAPPER_PATH}"; exit 1; }

    mkdir -p "${BASE_DIR}/skills/${EXPECTED_SKILL}" "${HOME_DIR}"
    printf 'name: agentsmesh\n' > "${BASE_DIR}/skills/${EXPECTED_SKILL}/SKILL.md"
    printf '{}\n' > "${BASE_DIR}/settings.json"

    require_contained "${BASE_DIR}"
    require_contained "${PROFILE_DIR}"
    require_contained "${HOME_DIR}"

    log_info "wrapper realpath: $(realpath "${WRAPPER_PATH}")"
    log_info "wrapper sha256: $(sha256sum "${WRAPPER_PATH}" | awk '{print $1}')"

    local output
    output="$(HOME="${HOME_DIR}" \
        PI_POD_LEAN_BASE_DIR="${BASE_DIR}" \
        PI_POD_LEAN_PROFILE_DIR="${PROFILE_DIR}" \
        PI_POD_SKILLS="${EXPECTED_SKILL}" \
        "${WRAPPER_PATH}" --dry-run -- --provider openai-codex --model gpt-5.5 -- 'preflight prompt')"

    [[ "${output}" == PI_CODING_AGENT_DIR=* ]] || { log_error "dry-run missing PI_CODING_AGENT_DIR prefix"; echo "${output}" >&2; exit 1; }
    [[ "${output}" == *" pi --no-skills "* ]] || { log_error "dry-run missing lean pi command"; echo "${output}" >&2; exit 1; }
    [[ "${output}" == *"--provider openai-codex"* ]] || { log_error "dry-run missing provider args"; echo "${output}" >&2; exit 1; }
    [[ "${output}" == *"--model gpt-5.5"* ]] || { log_error "dry-run missing model args"; echo "${output}" >&2; exit 1; }

    while IFS= read -r link; do
        [[ -L "${link}" ]] || continue
        local target
        target="$(realpath "${link}")"
        case "${target}" in
            "${TMP_DIR}"|"${TMP_DIR}"/*) ;;
            *) log_error "profile symlink escapes temp sandbox: ${link} -> ${target}"; exit 1 ;;
        esac
    done < <(find "${PROFILE_DIR}" -type l -print)

    log_info "preflight passed"
    exit 0
}

main "$@"
```

- [ ] **Step 2: Create the redacted daemon-state inspection script**

Create `tools/pi-lean/inspect-pod-daemon-redacted.sh`:

```bash
#!/bin/bash
set -euo pipefail

log_info()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO:  $*"; }
log_warn()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN:  $*" >&2; }
log_error() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2; }

cleanup() {
    local exit_code=$?
    [[ ${exit_code} -ne 0 ]] && log_error "Failed with exit code ${exit_code}"
}
trap 'cleanup' EXIT

main() {
    [[ $# -eq 1 ]] || { log_error "Usage: inspect-pod-daemon-redacted.sh <sandbox-path>"; exit 1; }
    local sandbox_path="$1"
    local state_path="${sandbox_path}/pod_daemon.json"
    [[ -f "${state_path}" ]] || { log_error "missing state file: ${state_path}"; exit 1; }

    python3 - "${state_path}" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    state = json.load(fh)

def redact_env(items):
    safe = []
    for item in items or []:
        key = item.split("=", 1)[0]
        if any(secret in key.upper() for secret in ("TOKEN", "KEY", "SECRET", "AUTH", "PASSWORD")):
            safe.append(f"{key}=<redacted>")
        elif key in ("PI_CODING_AGENT_DIR", "PI_POD_LEAN_BASE_DIR", "PI_POD_LEAN_PROFILE_DIR", "PI_POD_SKILLS", "HOME", "PATH"):
            safe.append(item)
    return safe

safe_state = {
    "pod_key": state.get("pod_key"),
    "agent": state.get("agent"),
    "sandbox_path": state.get("sandbox_path"),
    "work_dir": state.get("work_dir"),
    "command": state.get("command"),
    "args": state.get("args", []),
    "env": redact_env(state.get("env", [])),
    "perpetual": state.get("perpetual", False),
    "started_at": state.get("started_at"),
}
print(json.dumps(safe_state, indent=2, sort_keys=True))
PY
    exit 0
}

main "$@"
```

- [ ] **Step 3: Make both scripts executable**

Run:

```bash
chmod +x tools/pi-lean/preflight.sh tools/pi-lean/inspect-pod-daemon-redacted.sh
```

Expected: command exits 0.

- [ ] **Step 4: Run non-live script checks**

Run:

```bash
bash -n tools/pi-lean/preflight.sh
bash -n tools/pi-lean/inspect-pod-daemon-redacted.sh
tools/pi-lean/preflight.sh /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean
```

Expected:

```text
preflight passed
```

The command may print timestamped info lines and the wrapper hash before that final line. It must not create a pod, call AgentsMesh APIs, write DB state, rebuild images, or deploy.

- [ ] **Step 5: Add a local fixture check for redaction**

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
  "args": ["--skill", "agentsmesh", "--", "--provider", "openai-codex", "--", "hello"],
  "env": ["PI_CODING_AGENT_DIR=/tmp/pi-lean-check/pi-home", "OPENAI_API_KEY=secret", "AUTH_TOKEN=secret", "PI_POD_SKILLS=agentsmesh"],
  "perpetual": false,
  "started_at": "2026-06-24T00:00:00Z"
}
JSON
tools/pi-lean/inspect-pod-daemon-redacted.sh "${tmpdir}" | tee "${tmpdir}/redacted.json"
grep -q '<redacted>' "${tmpdir}/redacted.json"
! grep -q 'secret' "${tmpdir}/redacted.json"
rm -rf "${tmpdir}"
```

Expected: command exits 0.

- [ ] **Step 6: Commit Task 5**

```bash
git add tools/pi-lean/preflight.sh tools/pi-lean/inspect-pod-daemon-redacted.sh
git commit -m "chore(pi): add lean pod validation helpers"
```

If you add `tools/pi-lean/BUILD.bazel`, include it in the same commit.

---

### Task 6: Final non-live verification and execution handoff

**Files:**
- Modify only if Task 1 to Task 5 surfaced required doc updates: `docs/superpowers/specs/2026-06-24-pi-lean-pod-agent-design.md`
- Read-only verification: all files touched by Tasks 1 to 5

**Interfaces:**
- Consumes: completed Task 1 to Task 5 commits.
- Produces: a green, non-live implementation branch ready for adversarial code review, but still not authorized for live pod creation.

- [ ] **Step 1: Run focused tests**

Run:

```bash
bazel test //agentfile/merge:merge_test //agentfile/serialize:serialize_test
bazel test //backend/internal/service/agentpod:agentpod_test --test_filter=TestExtractAgentfileOverrides_PiLeanLayerSerializesRunnableArgs
bazel test //backend/internal/service/agent:agent_test --test_filter='TestConfigBuilder_PiLeanMergedSource.*'
bazel test //runner/internal/agents/pi:pi_test
bazel test //runner/internal/tokenusage:tokenusage_test --test_filter='Test(GetParser|Collect_PiLeanActualLaunchCommand|RegisteredParsers_HaveFixtureProducingNonZeroTokens|RegistryCoverage_EveryNonOptOutParserHasFixture)'
bazel test //runner/internal/runner:runner_test --test_filter='TestPodBuilderResolveLaunchInputs_PiLeanPromptDelimiter|TestPrepareAgentHome(RejectsCopiedSymlinkEscape|AllowsContainedSymlink)'
```

Expected: all PASS.

- [ ] **Step 2: Run package-level tests for modified areas**

Run:

```bash
bazel test //agentfile/...
bazel test //backend/internal/service/agent:agent_test //backend/internal/service/agentpod:agentpod_test
bazel test //runner/internal/agents/pi:pi_test //runner/internal/tokenusage:tokenusage_test //runner/internal/runner:runner_test
```

Expected: all PASS. If `//runner/internal/runner:runner_test` exposes unrelated pre-existing flakes, capture the exact failing test and run the focused Task 4 filters again before escalating.

- [ ] **Step 3: Run shell validations**

Run:

```bash
bash -n tools/pi-lean/preflight.sh
bash -n tools/pi-lean/inspect-pod-daemon-redacted.sh
tools/pi-lean/preflight.sh /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean
```

Expected: all commands exit 0; preflight prints `preflight passed`.

- [ ] **Step 4: Scan for forbidden placeholders and long dashes**

Run:

```bash
if grep -RInE 'T[B]D|T[O]DO|implement[[:space:]]+later|fill[[:space:]]+in[[:space:]]+details' docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md agentfile backend runner tools/pi-lean 2>/dev/null; then exit 1; fi
if grep -RInP '[\x{2013}\x{2014}]' docs/superpowers/plans/2026-06-24-pi-lean-pod-agent.md agentfile backend runner tools/pi-lean 2>/dev/null; then exit 1; fi
```

Expected: no output.

- [ ] **Step 5: Confirm no live operations happened**

Run:

```bash
git status --short
```

Expected: only intentional code, tests, docs, and tool files are modified or committed. There must be no backend/web image rebuild, no DB migration application, no pod creation artifact from the live AgentsMesh stack, and no deployment change.

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

1. Run `tools/pi-lean/preflight.sh /home/malfirg/programming_projects/pi-config/bin/pi-pod-lean`.
2. Resolve the laptop runner ID explicitly from the safe backend/UI path chosen by the operator. Do not auto-select a runner.
3. Create exactly one non-perpetual pod with fixed alias `pi-lean-validation-2026-06-24`, `agent_slug=pi-cli`, explicit `runner_id=<laptop runner id>`, and the canonical lean AgentFile layer.
4. Inspect daemon state only through `tools/pi-lean/inspect-pod-daemon-redacted.sh <sandbox-path>`.
5. Verify command, args, pre-exec env, workdir, `<sandbox>/pi-home`, and `<sandbox>/pi-lean-home/sessions`.
6. Verify token usage collection through the actual launch key.
7. Terminate the pod and remove only that pod's sandbox after token usage is collected.
8. If any check fails, terminate by pod key and inspect only redacted logs.

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
