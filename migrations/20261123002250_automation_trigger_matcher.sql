-- +goose Up
-- A version's Trigger is an Event matcher (ADR 0133): its action, the column renamed so that every
-- stored Trigger becomes a matcher without a filter, and the filter on the Event's own properties, a
-- jsonb object (ADR 0169), NULL for none. ent's diff wrote a drop and an add; it is a rename by hand,
-- with the index renamed alike, and the constraints of the new column added NOT VALID: the next
-- migration validates them.
ALTER TABLE "automation_versions" RENAME COLUMN "trigger_event" TO "trigger_action";
ALTER INDEX "automationversion_workspace_id_trigger_event" RENAME TO "automationversion_workspace_id_trigger_action";
ALTER TABLE "automation_versions" ADD COLUMN "trigger_filter" jsonb NULL;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_trigger_filter_check" CHECK (jsonb_typeof(trigger_filter) = 'object'::text) NOT VALID;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_trigger_filter_entries_check" CHECK ((trigger_filter IS NULL) OR COALESCE((jsonb_typeof((trigger_filter -> 'entries'::text)) = 'array'::text), false)) NOT VALID;

-- +goose Down
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_trigger_filter_entries_check", DROP CONSTRAINT "automation_versions_trigger_filter_check", DROP COLUMN "trigger_filter";
ALTER INDEX "automationversion_workspace_id_trigger_action" RENAME TO "automationversion_workspace_id_trigger_event";
ALTER TABLE "automation_versions" RENAME COLUMN "trigger_action" TO "trigger_event";
