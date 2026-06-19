-- Enforce one org-level label per (organization_id, name). The existing
-- labels_organization_id_repository_id_name_key UNIQUE is NULLS-distinct, so it
-- never constrained rows with repository_id IS NULL: get-or-create label logic
-- (backend ext API + the agentsmesh_backlog_sync.py mirror) could race two
-- identical org-level labels into existence (AMESH-5). A partial unique index
-- makes that duplicate impossible without touching repo-scoped labels.

CREATE UNIQUE INDEX IF NOT EXISTS labels_org_name_global_unique
    ON labels (organization_id, name)
    WHERE repository_id IS NULL;
