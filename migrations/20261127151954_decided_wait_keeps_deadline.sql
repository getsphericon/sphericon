-- +goose Up
-- A Wait for event on an open or a click that decided its path when the Enrollment reached it keeps
-- its deadline on its Step record all the same (ADR 0124): the check that forbade both goes.
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_decided_check";

-- +goose Down
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_decided_check" CHECK ((due_at IS NULL) OR (decided_path_id IS NULL)) NOT VALID;
