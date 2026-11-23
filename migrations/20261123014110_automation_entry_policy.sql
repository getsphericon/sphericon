-- +goose Up
-- An Automation version with an Event Trigger has an Entry policy, `once` or `again`, and a cooldown
-- of at least an hour (ADR 0127); a Contact may then have several Enrollments in an Automation, one
-- active at a time, each numbered by its ordinal, and an entry within the cooldown is skipped as
-- `cooldown`. ent's diff wrote the DDL; it is split by hand: this migration adds the columns, every
-- constraint NOT VALID, and the next one (20261123014111) validates them, builds the new unique
-- indexes concurrently and drops the one on the Automation and the Contact last.
-- The old unique index held one Enrollment per Contact and Automation, so each existing Enrollment
-- is its Contact's first by its beginning: the ordinal's default of 1 is the whole backfill, dropped
-- once the column holds it. ent's diff does not see a changed enum check, so the reasons' check is
-- replaced by hand.
ALTER TABLE "automation_runs" ADD COLUMN "ordinal" bigint NOT NULL DEFAULT 1, ADD COLUMN "key_value" character varying NULL;
ALTER TABLE "automation_runs" ALTER COLUMN "ordinal" DROP DEFAULT;
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_ordinal_check" CHECK (ordinal >= 1) NOT VALID;
ALTER TABLE "automation_versions" ADD COLUMN "entry_policy" character varying NOT NULL DEFAULT 'once', ADD COLUMN "cooldown_seconds" bigint NOT NULL DEFAULT 3600;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_cooldown_check" CHECK (cooldown_seconds >= 3600) NOT VALID;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_entry_policy_check" CHECK ((entry_policy)::text = ANY ((ARRAY['once'::character varying, 'again'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_skipped_entries" DROP CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_skipped_entries" ADD CONSTRAINT "automation_skipped_entries_reason_check" CHECK ((reason)::text = ANY ((ARRAY['cycle'::character varying, 'active_enrollment'::character varying, 'once'::character varying, 'cooldown'::character varying])::text[])) NOT VALID;

-- +goose Down
ALTER TABLE "automation_skipped_entries" DROP CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_skipped_entries" ADD CONSTRAINT "automation_skipped_entries_reason_check" CHECK ((reason)::text = ANY ((ARRAY['cycle'::character varying, 'active_enrollment'::character varying, 'once'::character varying])::text[]));
ALTER TABLE "automation_versions" DROP COLUMN "cooldown_seconds", DROP COLUMN "entry_policy", DROP CONSTRAINT "automation_versions_entry_policy_check", DROP CONSTRAINT "automation_versions_cooldown_check";
ALTER TABLE "automation_runs" DROP COLUMN "key_value", DROP COLUMN "ordinal", DROP CONSTRAINT "automation_runs_ordinal_check";
