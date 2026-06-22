-- 000160_add_pi_agent.up.sql
-- Register the pi coding agent (@earendil-works/pi-coding-agent) as a builtin.
--
-- slug='pi-cli' follows the claude-code / codex-cli convention, but
-- AGENT/EXECUTABLE/launch_command MUST be the real binary `pi` (the runner
-- exec()s LaunchCommand; the slug would ENOENT every pod -- see 000157).
--
-- PTY-only first pass (ACP deferred). pi authenticates via openai-codex OAuth
-- in ~/.pi/agent/auth.json, copied per-pod into PI_CODING_AGENT_DIR by the
-- runner's RegisterAgentHome spec. --provider/--model are pinned because a bare
-- `pi` defaults to provider `google`. OPENAI_API_KEY is OPTIONAL schema metadata
-- (the evaluator skips `ENV X SECRET OPTIONAL` at eval time) for the key-based
-- openai path; the primary auth path is the copied OAuth token.
INSERT INTO agents (slug, name, launch_command, executable, is_builtin, is_active, supported_modes, agentfile_source)
VALUES ('pi-cli', 'Pi', 'pi', 'pi', true, true, 'pty',
  E'# === Identity ===\nAGENT pi\nEXECUTABLE pi\n\n# === Mode ===\nMODE pty\n\n# === Configuration ===\nCONFIG model SELECT("gpt-5.5", "gpt-5.4", "gpt-5.3-codex-spark") = "gpt-5.5"\n\n# === Environment ===\nENV OPENAI_API_KEY SECRET OPTIONAL\nENV PI_CODING_AGENT_DIR = sandbox.root + "/pi-home"\n\n# === Prompt ===\nPROMPT_POSITION prepend\n\n# === Build Logic ===\narg "--provider" "openai-codex"\narg "--model" config.model when config.model != ""\n');
