-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: a Step record's outcome is done,
-- skipped or a path, and a path names the path taken (ADR 0124).
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_outcome_check";
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_skipped_check";

-- +goose Down
