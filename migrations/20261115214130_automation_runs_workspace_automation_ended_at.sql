-- +goose NO TRANSACTION
-- +goose Up
-- Backs an Automation's Results: its Enrollments that ended in a window.
CREATE INDEX CONCURRENTLY "automationrun_workspace_id_automation_id_ended_at" ON "automation_runs" ("workspace_id", "automation_id", "ended_at");

-- +goose Down
DROP INDEX CONCURRENTLY "automationrun_workspace_id_automation_id_ended_at";
