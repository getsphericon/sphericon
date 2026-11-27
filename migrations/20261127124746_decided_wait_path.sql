-- +goose Up
-- A Wait for event on an open or a click keeps the path it decided when the Enrollment reached it
-- (ADR 0124), and a decided wait has no deadline. ent's diff wrote the column and the check; the
-- check is added NOT VALID, and the next migration (20261127124747) validates it.
ALTER TABLE "automation_step_records" ADD COLUMN "decided_path_id" character varying NULL;
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_decided_check" CHECK ((due_at IS NULL) OR (decided_path_id IS NULL)) NOT VALID;

-- +goose Down
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_decided_check", DROP COLUMN "decided_path_id";
