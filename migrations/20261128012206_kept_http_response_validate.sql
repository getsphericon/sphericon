-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: an HTTP step's kept body goes with
-- its status and no reason to drop it, the reason is one of the closed list, and only a record that
-- is done keeps a response (ADR 0161).
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_http_body_check";
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_http_dropped_check";
ALTER TABLE "automation_step_records" VALIDATE CONSTRAINT "automation_step_records_skipped_check";

-- +goose Down
