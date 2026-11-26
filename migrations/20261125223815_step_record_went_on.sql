-- +goose Up
-- A Wait until's Step record keeps why the Enrollment went on at once when it had no moment ahead
-- within 366 days (ADR 0126): past, no_date or beyond_bound, never beside a due moment. The checks
-- are added NOT VALID, and the next migration validates them.
ALTER TABLE "automation_step_records" ADD COLUMN "went_on" character varying NULL;
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_went_on_check" CHECK ((went_on)::text = ANY ((ARRAY['past'::character varying, 'no_date'::character varying, 'beyond_bound'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_wait_check" CHECK ((due_at IS NULL) OR (went_on IS NULL)) NOT VALID;

-- +goose Down
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_wait_check";
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_went_on_check";
ALTER TABLE "automation_step_records" DROP COLUMN "went_on";
