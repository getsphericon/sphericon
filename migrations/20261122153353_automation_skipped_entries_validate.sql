-- +goose NO TRANSACTION
-- +goose Up
-- Validates the constraints of automation_runs the previous migration added NOT VALID and builds
-- the indexes of an Enrollment's Event and Firing concurrently.
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_entry_check";
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_rule_firings_firing";
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_workspace_automation_firing" ON "automation_runs" ("workspace_id", "automation_id", "firing_id");
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_workspace_automation_trigger_source" ON "automation_runs" ("workspace_id", "automation_id", "trigger_source_id");
CREATE INDEX CONCURRENTLY "automationrun_workspace_id_firing_id" ON "automation_runs" ("workspace_id", "firing_id");

-- +goose Down
DROP INDEX CONCURRENTLY "automationrun_workspace_id_firing_id";
DROP INDEX CONCURRENTLY "automation_runs_workspace_automation_trigger_source";
DROP INDEX CONCURRENTLY "automation_runs_workspace_automation_firing";
