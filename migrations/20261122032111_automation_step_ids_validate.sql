-- +goose NO TRANSACTION
-- +goose Up
-- Validates the constraints the previous migration added NOT VALID, and builds the indexes of the
-- version and the Step id an Automation message and a figure name (ADR 0122) concurrently.
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_provenance_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_automation_versions_automation_version";
ALTER TABLE "automation_figures" VALIDATE CONSTRAINT "automation_figures_automation_versions_version";
-- create index "outbound_messages_workspace_id_automation_version_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_automation_version_id" ON "outbound_messages" ("workspace_id", "automation_version_id");
-- create index "automation_figures_workspace_automation_version_step" to table: "automation_figures"
CREATE UNIQUE INDEX CONCURRENTLY "automation_figures_workspace_automation_version_step" ON "automation_figures" ("workspace_id", "automation_id", "version_id", "step_id");
-- create index "automation_figures_workspace_id_version_id" to table: "automation_figures"
CREATE INDEX CONCURRENTLY "automation_figures_workspace_id_version_id" ON "automation_figures" ("workspace_id", "version_id");

-- +goose Down
DROP INDEX CONCURRENTLY "automation_figures_workspace_id_version_id";
DROP INDEX CONCURRENTLY "automation_figures_workspace_automation_version_step";
DROP INDEX CONCURRENTLY "outbound_messages_workspace_id_automation_version_id";
