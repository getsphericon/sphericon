-- +goose Up
-- A Wait for event on an open or a click keeps the path it decided when the Enrollment reached it
-- (ADR 0124), beside its deadline.
ALTER TABLE "automation_step_records" ADD COLUMN "decided_path_id" character varying NULL;

-- +goose Down
ALTER TABLE "automation_step_records" DROP COLUMN "decided_path_id";
