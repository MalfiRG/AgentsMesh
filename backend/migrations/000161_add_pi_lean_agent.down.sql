-- 000161_add_pi_lean_agent.down.sql
-- Mirror of 000160 down: clear FK-referencing rows before removing the agents
-- row, in a transaction. organization_agent_configs / organization_agents hold
-- NO ACTION FKs onto agents(slug); user_agent_configs holds agent_slug without
-- an FK and is cleared to avoid orphan slug references.
BEGIN;

DELETE FROM organization_agent_configs WHERE agent_slug = 'pi-lean-cli';
DELETE FROM organization_agents       WHERE agent_slug = 'pi-lean-cli';
DELETE FROM user_agent_configs        WHERE agent_slug = 'pi-lean-cli';
DELETE FROM agents                    WHERE slug = 'pi-lean-cli';

COMMIT;
