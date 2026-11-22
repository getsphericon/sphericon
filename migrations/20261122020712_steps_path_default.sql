-- +goose Up
-- An Automation version's steps become a path (ADR 0122): an object {"id", "steps"} instead of an
-- array. The CHECK that they are an array goes before the Go migration that rewrites every stored
-- version (20261122020713), and the column loses its default, `[]`: every version is written with
-- its steps.
-- modify "automation_versions" table
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_steps_check";
ALTER TABLE "automation_versions" ALTER COLUMN "steps" DROP DEFAULT;

-- +goose Down
-- reverse: modify "automation_versions" table
ALTER TABLE "automation_versions" ALTER COLUMN "steps" SET DEFAULT '[]';
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_steps_check" CHECK (jsonb_typeof(steps) = 'array'::text) NOT VALID;
