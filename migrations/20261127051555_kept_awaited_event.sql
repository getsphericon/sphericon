-- +goose Up
-- A Wait for event's Step record keeps a copy of the Event it arrived on (ADR 0136): its id, action
-- and properties, set together, and only on a record that left down a path. ent's diff wrote the
-- columns and the new check; it does not see a changed variant check, so that one is replaced by
-- hand. Both are added NOT VALID, and the next migration (20261127051556) validates them.
ALTER TABLE "automation_step_records" ADD COLUMN "awaited_event_id" bigint NULL, ADD COLUMN "awaited_action" character varying NULL, ADD COLUMN "awaited_properties" jsonb NULL;
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_awaited_check" CHECK ((awaited_event_id IS NULL) = (awaited_action IS NULL)) NOT VALID;
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_skipped_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_skipped_check" CHECK ((((outcome)::text = 'done'::text) AND (skip_reason IS NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL) AND (awaited_event_id IS NULL) AND (awaited_action IS NULL) AND (awaited_properties IS NULL)) OR (((outcome)::text = 'skipped'::text) AND (skip_reason IS NOT NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL) AND (awaited_event_id IS NULL) AND (awaited_action IS NULL) AND (awaited_properties IS NULL)) OR (((outcome)::text = 'path'::text) AND (path_id IS NOT NULL) AND (skip_reason IS NULL))) NOT VALID;

-- +goose Down
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_skipped_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_skipped_check" CHECK ((((outcome)::text = 'done'::text) AND (skip_reason IS NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL)) OR (((outcome)::text = 'skipped'::text) AND (skip_reason IS NOT NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL)) OR (((outcome)::text = 'path'::text) AND (path_id IS NOT NULL) AND (skip_reason IS NULL)));
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_awaited_check", DROP COLUMN "awaited_properties", DROP COLUMN "awaited_action", DROP COLUMN "awaited_event_id";
