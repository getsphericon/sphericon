-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID: a Wait for event on an open or a click
-- either waits for a deadline or decided its path at once (ADR 0124).
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_decided_check";

-- +goose Down
