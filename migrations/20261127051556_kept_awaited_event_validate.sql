-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: a kept Event's id and action are set
-- together, and only a record that left down a path keeps one (ADR 0136).
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_awaited_check";
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_skipped_check";

-- +goose Down
