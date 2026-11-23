-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID, builds the unique indexes of a
-- Contact's one active Enrollment in an Automation per key value (a null key value one value) and
-- of its Enrollments' ordinals concurrently, and drops the unique index on the Automation and the
-- Contact, which allowed one Enrollment ever (ADR 0127), last.
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_ordinal_check";
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_cooldown_check";
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_entry_policy_check";
ALTER TABLE "automation_skipped_entries" VALIDATE CONSTRAINT "automation_skipped_entries_reason_check";
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_active_enrollment" ON "automation_runs" ("workspace_id", "automation_id", "contact_id", "key_value") NULLS NOT DISTINCT WHERE ((status)::text = 'active'::text);
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_workspace_automation_contact_ordinal" ON "automation_runs" ("workspace_id", "automation_id", "contact_id", "ordinal");
DROP INDEX CONCURRENTLY "automation_runs_workspace_id_automation_id_contact_id";

-- +goose Down
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_workspace_id_automation_id_contact_id" ON "automation_runs" ("workspace_id", "automation_id", "contact_id");
DROP INDEX CONCURRENTLY "automation_runs_workspace_automation_contact_ordinal";
DROP INDEX CONCURRENTLY "automation_runs_active_enrollment";
