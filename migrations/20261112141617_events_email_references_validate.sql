-- +goose NO TRANSACTION
-- +goose Up
-- A reference to no row of its Workspace was never written by the app; one would fail the
-- validation, so it is cleared first.
UPDATE "events" SET "automation_id" = NULL WHERE "automation_id" IS NOT NULL AND NOT EXISTS (SELECT 1 FROM "automations" a WHERE a."workspace_id" = "events"."workspace_id" AND a."id" = "events"."automation_id");
UPDATE "events" SET "broadcast_id" = NULL WHERE "broadcast_id" IS NOT NULL AND NOT EXISTS (SELECT 1 FROM "broadcasts" b WHERE b."workspace_id" = "events"."workspace_id" AND b."id" = "events"."broadcast_id");
CREATE INDEX CONCURRENTLY "event_workspace_id_automation_id" ON "events" ("workspace_id", "automation_id") WHERE (automation_id IS NOT NULL);
CREATE INDEX CONCURRENTLY "event_workspace_id_broadcast_id" ON "events" ("workspace_id", "broadcast_id") WHERE (broadcast_id IS NOT NULL);
ALTER TABLE "events" VALIDATE CONSTRAINT "events_automations_automation";
ALTER TABLE "events" VALIDATE CONSTRAINT "events_broadcasts_broadcast";

-- +goose Down
DROP INDEX CONCURRENTLY "event_workspace_id_broadcast_id";
DROP INDEX CONCURRENTLY "event_workspace_id_automation_id";
