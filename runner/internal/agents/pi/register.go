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
