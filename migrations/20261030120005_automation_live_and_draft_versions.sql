-- +goose NO TRANSACTION
-- +goose Up
-- An Automation version holds the Trigger and the Integration, and the Automation points at its
-- live version and its draft (ADR 0065). Written by ent's diff, with the keys added NOT VALID then
-- validated and the indexes built concurrently (ADR 0033). The Automation's own trigger_event loses
-- its NOT NULL: it is no longer written. The data moves in the next migration.
-- modify "automation_versions" table
ALTER TABLE "automation_versions" ADD COLUMN "trigger_event" character varying NULL, ADD COLUMN "integration_id" bigint NULL;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_integrations_integration";
-- create index "automationversion_integration_id" to table: "automation_versions"
CREATE INDEX CONCURRENTLY "automationversion_integration_id" ON "automation_versions" ("integration_id");
-- create index "automationversion_workspace_id_trigger_event" to table: "automation_versions"
CREATE INDEX CONCURRENTLY "automationversion_workspace_id_trigger_event" ON "automation_versions" ("workspace_id", "trigger_event");
-- modify "automations" table
ALTER TABLE "automations" ALTER COLUMN "trigger_event" DROP NOT NULL, ADD COLUMN "live_version_id" bigint NULL, ADD COLUMN "draft_version_id" bigint NULL;
ALTER TABLE "automations" ADD CONSTRAINT "automations_automation_versions_draft_version" FOREIGN KEY ("draft_version_id") REFERENCES "automation_versions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_automation_versions_draft_version";
ALTER TABLE "automations" ADD CONSTRAINT "automations_automation_versions_live_version" FOREIGN KEY ("live_version_id") REFERENCES "automation_versions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_automation_versions_live_version";
-- create index "automation_draft_version_id" to table: "automations"
CREATE INDEX CONCURRENTLY "automation_draft_version_id" ON "automations" ("draft_version_id");
-- create index "automation_live_version_id" to table: "automations"
CREATE INDEX CONCURRENTLY "automation_live_version_id" ON "automations" ("live_version_id");

-- +goose Down
-- reverse: create index "automation_live_version_id" to table: "automations"
DROP INDEX CONCURRENTLY "automation_live_version_id";
-- reverse: create index "automation_draft_version_id" to table: "automations"
DROP INDEX CONCURRENTLY "automation_draft_version_id";
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP CONSTRAINT "automations_automation_versions_live_version", DROP CONSTRAINT "automations_automation_versions_draft_version", DROP COLUMN "draft_version_id", DROP COLUMN "live_version_id";
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "automations" ALTER COLUMN "trigger_event" SET NOT NULL;
-- reverse: create index "automationversion_workspace_id_trigger_event" to table: "automation_versions"
DROP INDEX CONCURRENTLY "automationversion_workspace_id_trigger_event";
-- reverse: create index "automationversion_integration_id" to table: "automation_versions"
DROP INDEX CONCURRENTLY "automationversion_integration_id";
-- reverse: modify "automation_versions" table
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_integrations_integration", DROP COLUMN "integration_id", DROP COLUMN "trigger_event";
