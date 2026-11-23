-- +goose NO TRANSACTION
-- +goose Up
-- Validates the constraints of the Trigger's filter the previous migration added NOT VALID.
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_trigger_filter_check";
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_trigger_filter_entries_check";

-- +goose Down
-- Nothing to undo: a validation changes no definition.
