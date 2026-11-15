-- +goose NO TRANSACTION
-- +goose Up
-- Backs an Automation's Results: its Enrollments that began in a window.
CREATE INDEX CONCURRENTLY "automationrun_workspace_id_automation_id_began_at" ON "automation_runs" ("workspace_id", "automation_id", "began_at");

-- +goose Down
DROP INDEX CONCURRENTLY "automationrun_workspace_id_automation_id_began_at";
