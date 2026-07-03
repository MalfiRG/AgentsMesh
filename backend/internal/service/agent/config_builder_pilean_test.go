package agent

import (
	"testing"

	"github.com/anthropics/agentsmesh/agentfile/eval"
	"github.com/anthropics/agentsmesh/agentfile/merge"
	"github.com/anthropics/agentsmesh/agentfile/parser"
	"github.com/anthropics/agentsmesh/agentfile/resolve"
	"github.com/anthropics/agentsmesh/agentfile/serialize"
	runnerv1 "github.com/anthropics/agentsmesh/proto/gen/go/runner/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// piCLIBaseAgentfile mirrors the builtin pi-cli agentfile_source seeded by
// migration 000160. The lean overlay is validated against this base.
const piCLIBaseAgentfile = `AGENT pi
EXECUTABLE pi
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
PROMPT_POSITION prepend
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
`

// piLeanLayer is the AgentFile-layer overlay that switches pi-cli to the lean
// wrapper with pod-local profile dirs (see the lean-Pi pod design spec).
const piLeanLayer = `AGENT pi-pod-lean
EXECUTABLE pi-pod-lean
ENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"
PROMPT_POSITION append
arg "--"
`

// evalMergedLayer replays the production path: merge base+layer
// (agentfile_extract.go), then eval to a CreatePodCommand (config_builder_*.go).
// The eval BuildResult is returned alongside for fields not carried into the
// proto (e.g. Executable, which is agent metadata not a launch instruction).
func evalMergedLayer(t *testing.T, baseSrc, layerSrc string, ticketLabels ...string) (*runnerv1.CreatePodCommand, *eval.BuildResult) {
	t.Helper()

	baseProg, errs := parser.Parse(baseSrc)
	require.Empty(t, errs, "base parse")
	layerProg, errs := parser.Parse(layerSrc)
	require.Empty(t, errs, "layer parse")

	layerConfigNames := resolve.ExtractConfigNames(layerProg)
	merge.Merge(baseProg, layerProg)
	resolve.ResolveConfigValues(baseProg, layerConfigNames, nil, nil)
	merged := serialize.Serialize(baseProg)

	prog, errs := parser.Parse(merged)
	require.Empty(t, errs, "merged parse")

	req := &ConfigBuildRequest{PodKey: "pod-1", MCPPort: 9000, TicketLabels: ticketLabels}
	ctx := buildEvalContext(req, map[string]interface{}{}, map[string]interface{}{}, nil)
	require.NoError(t, eval.Eval(prog, ctx))
	eval.ApplyModeArgs(ctx.Result)
	eval.ApplyRemoves(ctx.Result)
	return buildResultToProto(req, ctx.Result), ctx.Result
}

// piLeanDurableAgentfile mirrors the agentfile_source seeded for the durable
// pi-lean-cli agent by migration 000161. Keep in sync with that migration.
const piLeanDurableAgentfile = `AGENT pi-pod-lean
EXECUTABLE pi-pod-lean
MODE pty
CONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"
ENV OPENAI_API_KEY SECRET OPTIONAL
ENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"
ENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"
ENV PI_POD_LABELS = str_join(ticket.labels, ",") when len(ticket.labels) != 0
PROMPT_POSITION append
arg "--provider" "openai-codex"
arg "--model" config.model when config.model != ""
arg "--"
`

// TestAgentfilePiLeanDurable_EvalsToLeanSpec guards the migration-seeded agent:
// its baked agentfile_source alone (no user layer) must eval to the lean spec.
// leanLaunchArgs is the exact ordered LaunchArgs the lean wrapper requires:
// wrapper flags, then the -- delimiter last so the appended prompt is a
// positional (not parsed as a flag). Order is asserted, not just membership.
var leanLaunchArgs = []string{"--provider", "openai-codex", "--model", "gpt-5.5", "--"}

func TestAgentfilePiLeanDurable_EvalsToLeanSpec(t *testing.T) {
	cmd, res := evalMergedLayer(t, piLeanDurableAgentfile, "MODE pty\n")

	assert.Equal(t, "pi-pod-lean", cmd.LaunchCommand)
	assert.Equal(t, "pi-pod-lean", res.Executable, "executable metadata must not stay pi")
	root := PlaceholderSandboxRoot
	assert.Equal(t, root+"/pi-home", cmd.EnvVars["PI_CODING_AGENT_DIR"])
	assert.Equal(t, root+"/pi-home", cmd.EnvVars["PI_POD_LEAN_BASE_DIR"])
	assert.Equal(t, root+"/pi-lean-home", cmd.EnvVars["PI_POD_LEAN_PROFILE_DIR"])
	assert.Equal(t, "append", cmd.PromptPosition)
	assert.Equal(t, leanLaunchArgs, cmd.LaunchArgs)

	_, hasLabels := cmd.EnvVars["PI_POD_LABELS"]
	assert.False(t, hasLabels, "no ticket -> PI_POD_LABELS must not be set")
}

func TestAgentfilePiLeanDurable_MapsTicketLabelsToPodLabels(t *testing.T) {
	cmd, _ := evalMergedLayer(t, piLeanDurableAgentfile, "MODE pty\n", "pi-skills:agentsmesh", "bug")
	assert.Equal(t, "pi-skills:agentsmesh,bug", cmd.EnvVars["PI_POD_LABELS"],
		"ticket labels join into PI_POD_LABELS for the lean wrapper")
}

// TestAgentfilePiLean_OmitsModelWhenUnset covers the empty branch of
// `arg "--model" config.model when config.model != ""`: an unset model must
// drop the flag entirely, not emit `--model ""`.
func TestAgentfilePiLean_OmitsModelWhenUnset(t *testing.T) {
	base := "AGENT pi-pod-lean\nCONFIG model = \"\"\narg \"--provider\" \"openai-codex\"\narg \"--model\" config.model when config.model != \"\"\narg \"--\"\n"
	cmd, _ := evalMergedLayer(t, base, "MODE pty\n")
	assert.NotContains(t, cmd.LaunchArgs, "--model", "empty model must omit the flag")
	assert.Equal(t, []string{"--provider", "openai-codex", "--"}, cmd.LaunchArgs)
}

func TestAgentfileLeanLayer_ProducesLeanLaunchSpec(t *testing.T) {
	cmd, res := evalMergedLayer(t, piCLIBaseAgentfile, piLeanLayer)

	assert.Equal(t, "pi-pod-lean", cmd.LaunchCommand, "AGENT override switches launch command")
	assert.Equal(t, "pi-pod-lean", res.Executable, "EXECUTABLE override must reach eval result")

	root := PlaceholderSandboxRoot
	assert.Equal(t, root+"/pi-home", cmd.EnvVars["PI_CODING_AGENT_DIR"], "inherited from base")
	assert.Equal(t, root+"/pi-home", cmd.EnvVars["PI_POD_LEAN_BASE_DIR"])
	assert.Equal(t, root+"/pi-lean-home", cmd.EnvVars["PI_POD_LEAN_PROFILE_DIR"])

	assert.Equal(t, "append", cmd.PromptPosition, "layer flips base prepend -> append")
	assert.Equal(t, leanLaunchArgs, cmd.LaunchArgs, "exact ordered wrapper args, -- last")
}
