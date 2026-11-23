-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: an Enrollment's chain is a list, and
-- a skipped entry's reason is one of the entry checks' (ADR 0127).
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_chain_check";
ALTER TABLE "automation_skipped_entries" VALIDATE CONSTRAINT "automation_skipped_entries_reason_check";

-- +goose Down
