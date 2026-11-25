-- +goose Up
-- A fork's Step record keeps the path it sent the Enrollment down and, at a Branch, each path's
-- outcome (ADR 0124): the outcome `path`, which requires path_id. ent's diff wrote the columns; it
-- does not see a changed enum or variant check, so both are replaced by hand, added NOT VALID, and
-- the next migration (20261125020654) validates them.
ALTER TABLE "automation_step_records" ADD COLUMN "path_id" character varying NULL, ADD COLUMN "path_outcomes" jsonb NULL;
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_outcome_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_outcome_check" CHECK ((outcome)::text = ANY ((ARRAY['done'::character varying, 'skipped'::character varying, 'path'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_skipped_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_skipped_check" CHECK ((((outcome)::text = 'done'::text) AND (skip_reason IS NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL)) OR (((outcome)::text = 'skipped'::text) AND (skip_reason IS NOT NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL)) OR (((outcome)::text = 'path'::text) AND (path_id IS NOT NULL) AND (skip_reason IS NULL))) NOT VALID;

-- +goose Down
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_skipped_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_skipped_check" CHECK ((((outcome)::text = 'done'::text) AND (skip_reason IS NULL)) OR (((outcome)::text = 'skipped'::text) AND (skip_reason IS NOT NULL)));
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_outcome_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_outcome_check" CHECK ((outcome)::text = ANY ((ARRAY['done'::character varying, 'skipped'::character varying])::text[]));
ALTER TABLE "automation_step_records" DROP COLUMN "path_outcomes", DROP COLUMN "path_id";
