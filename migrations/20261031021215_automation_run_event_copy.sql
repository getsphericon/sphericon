-- +goose Up
-- modify "automation_runs" table
ALTER TABLE "automation_runs" ADD COLUMN "event_action" character varying NULL, ADD COLUMN "event_properties" jsonb NULL;

-- +goose Down
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" DROP COLUMN "event_properties", DROP COLUMN "event_action";
