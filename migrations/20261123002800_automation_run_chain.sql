-- +goose Up
-- An Enrollment keeps the Automations the chain that made it passed through, and an entry whose
-- cause already passed through the Automation is skipped as `cycle` (ADR 0127). ent's diff wrote
-- the column and its check; the checks are added NOT VALID, and the next migration
-- (20261123002801) validates them. ent's diff does not see a changed enum check, so the reasons'
-- check is replaced by hand.
ALTER TABLE "automation_runs" ADD COLUMN "chain" jsonb NOT NULL DEFAULT '[]';
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_chain_check" CHECK (jsonb_typeof(chain) = 'array'::text) NOT VALID;
ALTER TABLE "automation_skipped_entries" DROP CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_skipped_entries" ADD CONSTRAINT "automation_skipped_entries_reason_check" CHECK ((reason)::text = ANY ((ARRAY['cycle'::character varying, 'active_enrollment'::character varying, 'once'::character varying])::text[])) NOT VALID;

-- +goose Down
ALTER TABLE "automation_skipped_entries" DROP CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_skipped_entries" ADD CONSTRAINT "automation_skipped_entries_reason_check" CHECK ((reason)::text = ANY ((ARRAY['active_enrollment'::character varying, 'once'::character varying])::text[]));
ALTER TABLE "automation_runs" DROP COLUMN "chain", DROP CONSTRAINT "automation_runs_chain_check";
