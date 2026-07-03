-- 000161_add_pi_lean_agent.up.sql
-- Register the lean-Pi wrapper (pi-pod-lean) as a builtin agent, promoting the
-- AgentFile-layer prototype validated in the lean-Pi pod design to a first-class
-- selectable agent. Rollback path is selecting pi-cli; this row is additive.
--
-- launch_command/executable MUST be `pi-pod-lean` (the runner exec()s
-- LaunchCommand; it resolves from the runner PATH -- laptop runner only, the
-- Hetzner runner does not ship the wrapper). The wrapper runs `pi --no-skills`
-- under a pod-local profile dir, so startup context is reduced vs pi-cli while
-- the capability surface is unchanged.
--
-- PI_CODING_AGENT_DIR lets RegisterAgentHome copy the host ~/.pi/agent into
-- <sandbox>/pi-home; PI_POD_LEAN_BASE_DIR reads from that copy; the runtime lean
-- profile is pod-local at PI_POD_LEAN_PROFILE_DIR=<sandbox>/pi-lean-home. Token
-- accounting for launch_command 'pi-pod-lean' is handled by the runner parser
-- alias + dual-root scan (pi-home + pi-lean-home). PROMPT_POSITION is append and
-- the trailing `arg "--"` guards a prompt that begins with a wrapper-looking flag.
INSERT INTO agents (slug, name, launch_command, executable, is_builtin, is_active, supported_modes, agentfile_source)
VALUES ('pi-lean-cli', 'Pi (lean)', 'pi-pod-lean', 'pi-pod-lean', true, true, 'pty',
  E'# === Identity ===\nAGENT pi-pod-lean\nEXECUTABLE pi-pod-lean\n\n# === Mode ===\nMODE pty\n\n# === Configuration ===\nCONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"\n\n# === Environment ===\nENV OPENAI_API_KEY SECRET OPTIONAL\nENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"\nENV PI_POD_LEAN_BASE_DIR = sandbox.root + "/pi-home"\nENV PI_POD_LEAN_PROFILE_DIR = sandbox.root + "/pi-lean-home"\nENV PI_POD_LABELS = str_join(ticket.labels, ",") when len(ticket.labels) != 0\n\n# === Prompt ===\nPROMPT_POSITION append\n\n# === Build Logic ===\narg "--provider" "openai-codex"\narg "--model" config.model when config.model != ""\narg "--"\n');
