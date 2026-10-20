-- +goose NO TRANSACTION
-- +goose Up
-- create index "automationrun_version_id" to table: "automation_runs"
CREATE INDEX CONCURRENTLY "automationrun_version_id" ON "automation_runs" ("version_id");

-- +goose Down
-- reverse: create index "automationrun_version_id" to table: "automation_runs"
DROP INDEX CONCURRENTLY "automationrun_version_id";
