-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: an Enrollment key is no empty name,
-- and a skipped entry's reason is one of the entry checks' (ADR 0127, ADR 0165).
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_enrollment_key_check";
ALTER TABLE "automation_skipped_entries" VALIDATE CONSTRAINT "automation_skipped_entries_reason_check";

-- +goose Down
