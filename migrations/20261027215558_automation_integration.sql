-- +goose NO TRANSACTION
-- +goose Up
-- modify "automations" table
ALTER TABLE "automations" ADD COLUMN "integration_id" bigint NULL;
ALTER TABLE "automations" ADD CONSTRAINT "automations_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_integrations_integration";
-- create index "automation_integration_id" to table: "automations"
CREATE INDEX CONCURRENTLY "automation_integration_id" ON "automations" ("integration_id");

-- +goose Down
-- reverse: create index "automation_integration_id" to table: "automations"
DROP INDEX CONCURRENTLY "automation_integration_id";
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP CONSTRAINT "automations_integrations_integration", DROP COLUMN "integration_id";
