-- +goose Up
-- An Automation version with an Event Trigger may name an Enrollment key, a property of the
-- triggering Event by whose value a Contact may have several active Enrollments at once (ADR 0165);
-- an entry whose Event lacks the property is skipped as `no_enrollment_key`, and one past a keyed
-- version's ceiling per Contact as `enrollment_ceiling`. ent's diff wrote the column and its check;
-- the check is added NOT VALID and the next migration (20261123031305) validates it. ent's diff does
-- not see a changed enum check, so the reasons' check is replaced by hand, every reason kept.
ALTER TABLE "automation_versions" ADD COLUMN "enrollment_key" character varying NULL;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_enrollment_key_check" CHECK ((enrollment_key IS NULL) OR ((enrollment_key)::text <> ''::text)) NOT VALID;
ALTER TABLE "automation_skipped_entries" DROP CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_skipped_entries" ADD CONSTRAINT "automation_skipped_entries_reason_check" CHECK ((reason)::text = ANY ((ARRAY['no_enrollment_key'::character varying, 'cycle'::character varying, 'active_enrollment'::character varying, 'once'::character varying, 'cooldown'::character varying, 'enrollment_ceiling'::character varying])::text[])) NOT VALID;

-- +goose Down
ALTER TABLE "automation_skipped_entries" DROP CONSTRAINT "automation_skipped_entries_reason_check";
ALTER TABLE "automation_skipped_entries" ADD CONSTRAINT "automation_skipped_entries_reason_check" CHECK ((reason)::text = ANY ((ARRAY['cycle'::character varying, 'active_enrollment'::character varying, 'once'::character varying, 'cooldown'::character varying])::text[]));
ALTER TABLE "automation_versions" DROP COLUMN "enrollment_key", DROP CONSTRAINT "automation_versions_enrollment_key_check";
