-- +goose NO TRANSACTION
-- +goose Up
-- Validates the constraints of the exit and goal matchers the previous migration added NOT VALID.
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_exits_check";
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_goal_check";

-- +goose Down
-- Nothing to undo: a validation changes no definition.
