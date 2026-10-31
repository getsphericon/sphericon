-- +goose Up
-- modify "automation_runs" table
ALTER TABLE "automation_runs" ADD COLUMN "ended_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" DROP COLUMN "ended_at";
