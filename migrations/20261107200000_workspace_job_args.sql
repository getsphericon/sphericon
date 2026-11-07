-- +goose Up
-- Every job of a Workspace names it at the args' top-level workspaceId, where the erase of the
-- Workspace finds and deletes it. Written by hand: ent's diff does not see the river_job rows.
--
-- The trigger and rule evaluations named it at workspace_id: their waiting jobs are rewritten in
-- place, so a job queued by the previous release is worked in its Workspace. A running job is
-- left as it is: the release that runs it reads the old key.
UPDATE "river_job"
SET "args" = ("args" - 'workspace_id') || jsonb_build_object('workspaceId', "args" -> 'workspace_id')
WHERE "kind" IN ('automation_evaluate_trigger', 'rule_evaluate_event')
  AND "state" <> 'running'
  AND "args" ? 'workspace_id';

-- A Workspace erased by an earlier release left its jobs in the queue: the ones that name it (an
-- event's envelope, the evaluations above) go now, as the erase deletes them from this release on.
-- A job that named only a row id (a delivery, a step, a send) cannot be traced to its Workspace;
-- the queue's retention removes it (a day once completed, 30 days once discarded).
DELETE FROM "river_job" j
USING "workspaces" w
WHERE w."erased_at" IS NOT NULL
  AND j."state" <> 'running'
  AND j."args" @> jsonb_build_object('workspaceId', w."id");

-- +goose Down
UPDATE "river_job"
SET "args" = ("args" - 'workspaceId') || jsonb_build_object('workspace_id', "args" -> 'workspaceId')
WHERE "kind" IN ('automation_evaluate_trigger', 'rule_evaluate_event')
  AND "state" <> 'running'
  AND "args" ? 'workspaceId';
