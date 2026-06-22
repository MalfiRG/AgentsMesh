-- 000160_add_pi_agent.down.sql
-- Clear FK-referencing rows before removing the agents row, in a transaction.
-- organization_agents / organization_agent_configs hold NO ACTION FKs onto
-- agents(slug) (000093); user_agent_configs holds agent_slug without an FK
-- (000090) and is cleared to avoid orphan slug references.
BEGIN;

DELETE FROM organization_agent_configs WHERE agent_slug = 'pi-cli';
DELETE FROM organization_agents       WHERE agent_slug = 'pi-cli';
DELETE FROM user_agent_configs        WHERE agent_slug = 'pi-cli';
DELETE FROM agents                    WHERE slug = 'pi-cli';

COMMIT;
