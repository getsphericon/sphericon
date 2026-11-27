-- +goose Up
-- A Wait for event's Step record keeps the Events its wake found the Condition false for (ADR
-- 0124), so its timeout never takes `arrived` on one of them.
-- modify "automation_step_records" table
ALTER TABLE "automation_step_records" ADD COLUMN "declined_event_ids" jsonb NULL;

-- +goose Down
-- reverse: modify "automation_step_records" table
ALTER TABLE "automation_step_records" DROP COLUMN "declined_event_ids";
