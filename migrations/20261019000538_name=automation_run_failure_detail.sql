-- +goose Up
-- modify "automation_runs" table
ALTER TABLE "automation_runs" ADD COLUMN "failure_kind" character varying NULL, ADD COLUMN "failure_status" bigint NULL, ADD COLUMN "failure_excerpt" character varying NULL;

-- +goose Down
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" DROP COLUMN "failure_excerpt", DROP COLUMN "failure_status", DROP COLUMN "failure_kind";
