-- +goose NO TRANSACTION
-- +goose Up
-- The reason now has a third value, so its enum check is added again (the previous migration dropped the old one); a disabled endpoint always
-- has a reason and an enabled one never does.
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD CONSTRAINT "webhook_endpoints_disabled_reason_check" CHECK ((disabled_reason)::text = ANY ((ARRAY['owner'::character varying, 'gone'::character varying, 'persistent_failure'::character varying])::text[])) NOT VALID, ADD CONSTRAINT "webhook_endpoints_disabled_check" CHECK ((disabled_at IS NULL) = (disabled_reason IS NULL)) NOT VALID;
ALTER TABLE "webhook_endpoints" VALIDATE CONSTRAINT "webhook_endpoints_disabled_reason_check";
ALTER TABLE "webhook_endpoints" VALIDATE CONSTRAINT "webhook_endpoints_disabled_check";
-- create index "webhookendpoint_workspace_id_disabled_at" to table: "webhook_endpoints"
CREATE INDEX CONCURRENTLY "webhookendpoint_workspace_id_disabled_at" ON "webhook_endpoints" ("workspace_id", "disabled_at");

-- +goose Down
-- reverse: create index "webhookendpoint_workspace_id_disabled_at" to table: "webhook_endpoints"
DROP INDEX CONCURRENTLY "webhookendpoint_workspace_id_disabled_at";
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" DROP CONSTRAINT "webhook_endpoints_disabled_check";
ALTER TABLE "webhook_endpoints" DROP CONSTRAINT "webhook_endpoints_disabled_reason_check";
