-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID, and builds the indexes of a date
-- Trigger's occurrences (ADR 0126) concurrently: one Enrollment, one deciding skipped entry and one
-- send-locked skipped entry per Automation, Contact and occurrence, and the date scheduler's read of
-- the versions with a date Trigger.
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_entry_check";
ALTER TABLE "automation_skipped_entries" VALIDATE CONSTRAINT "automation_skipped_entries_entry_check";
ALTER TABLE "automation_skipped_entries" VALIDATE CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_trigger_date_check";
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_trigger_kind_check";
ALTER TABLE "contacts" VALIDATE CONSTRAINT "contacts_send_locked_dates_check";
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_occurrence" ON "automation_runs" ("workspace_id", "automation_id", "contact_id", "occurrence") WHERE (occurrence IS NOT NULL);
CREATE UNIQUE INDEX CONCURRENTLY "automation_skipped_entries_occurrence" ON "automation_skipped_entries" ("workspace_id", "automation_id", "contact_id", "occurrence") WHERE ((occurrence IS NOT NULL) AND ((reason)::text <> 'send_locked'::text));
CREATE UNIQUE INDEX CONCURRENTLY "automation_skipped_entries_occurrence_send_locked" ON "automation_skipped_entries" ("workspace_id", "automation_id", "contact_id", "occurrence") WHERE ((occurrence IS NOT NULL) AND ((reason)::text = 'send_locked'::text));
CREATE INDEX CONCURRENTLY "automation_versions_date_trigger" ON "automation_versions" ("workspace_id", "automation_id") WHERE (trigger_date IS NOT NULL);

-- +goose Down
DROP INDEX CONCURRENTLY "automation_versions_date_trigger";
DROP INDEX CONCURRENTLY "automation_skipped_entries_occurrence_send_locked";
DROP INDEX CONCURRENTLY "automation_skipped_entries_occurrence";
DROP INDEX CONCURRENTLY "automation_runs_occurrence";
