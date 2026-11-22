-- +goose Up
-- An Automation version's steps are a path holding its Steps as a list (ADR 0122): the array a
-- release before the path writes is refused. It sorts after the Go migration that rewrites every
-- stored version (20261122020713). The CHECK is added NOT VALID under the runner's bounded lock
-- wait, and validated by the next migration.
-- modify "automation_versions" table
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_steps_check" CHECK (COALESCE((jsonb_typeof((steps -> 'steps'::text)) = 'array'::text), false)) NOT VALID;

-- +goose Down
-- reverse: modify "automation_versions" table
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_steps_check";
