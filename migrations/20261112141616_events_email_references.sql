-- +goose Up
-- An Event's Broadcast and Automation are foreign keys, which a Workspace's erase alone deletes:
-- added NOT VALID here, under the runner's bounded lock wait, and validated by the next migration.
ALTER TABLE "events" ADD CONSTRAINT "events_automations_automation" FOREIGN KEY ("workspace_id", "automation_id") REFERENCES "automations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (automation_id) NOT VALID;
ALTER TABLE "events" ADD CONSTRAINT "events_broadcasts_broadcast" FOREIGN KEY ("workspace_id", "broadcast_id") REFERENCES "broadcasts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (broadcast_id) NOT VALID;

-- +goose Down
ALTER TABLE "events" DROP CONSTRAINT "events_broadcasts_broadcast", DROP CONSTRAINT "events_automations_automation";
