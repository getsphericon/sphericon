-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID.
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_condition_entries_check";
ALTER TABLE "rule_versions" VALIDATE CONSTRAINT "rule_versions_condition_entries_check";
ALTER TABLE "segments" VALIDATE CONSTRAINT "segments_condition_entries_check";

-- +goose Down
