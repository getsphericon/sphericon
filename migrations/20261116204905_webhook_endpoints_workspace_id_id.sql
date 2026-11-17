-- +goose NO TRANSACTION
-- +goose Up
-- The key the health of an endpoint references (ADR 0017, ADR 0168), built concurrently before the
-- table that references it is made (20261116204906_job_state_tables).
CREATE UNIQUE INDEX CONCURRENTLY "webhook_endpoints_workspace_id_id" ON "webhook_endpoints" ("workspace_id", "id");

-- +goose Down
DROP INDEX CONCURRENTLY "webhook_endpoints_workspace_id_id";
