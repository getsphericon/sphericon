-- +goose NO TRANSACTION
-- +goose Up
-- A row that fails is a document the app could not decode already; it is left for a person, never
-- rewritten (an emptied Condition would match every Contact).
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_steps_check";
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_condition_check";
ALTER TABLE "rule_versions" VALIDATE CONSTRAINT "rule_versions_actions_check";
ALTER TABLE "rule_versions" VALIDATE CONSTRAINT "rule_versions_condition_check";
ALTER TABLE "segments" VALIDATE CONSTRAINT "segments_condition_check";

-- +goose Down
SELECT 1;
