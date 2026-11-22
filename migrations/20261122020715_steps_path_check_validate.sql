-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID.
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_steps_check";

-- +goose Down
