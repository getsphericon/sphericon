-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: a Wait until's reason for going on
-- at once is one of its three, and never beside a due moment (ADR 0126).
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_went_on_check";
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_wait_check";

-- +goose Down
