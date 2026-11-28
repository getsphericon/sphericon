-- +goose Up
-- An HTTP step's Step record keeps the response it got (ADR 0161): its status and, when it was a
-- JSON object it could keep, its body, else why it dropped it, only on a record that is done. ent's
-- diff wrote the columns and the new checks; it does not see a changed variant check, so that one
-- is replaced by hand. The checks are added NOT VALID, and the next migration (20261128012206)
-- validates them. An HTTP step stored before has no name, which no save could give it now (a name
-- is fixed when its Step is created); the project carries no legacy (ADR 0047), so a database that
-- holds one is refused first, to be reset (`mise run db:reset`). It rides Major 4, the Major of the
-- collapsed history, which no release has shipped.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "automation_versions" WHERE jsonb_path_exists("steps", 'lax $.** ? (@.type == "http" && !exists(@.name))')) THEN
    RAISE EXCEPTION 'the database holds HTTP steps with no name (ADR 0161): reset it, the project carries no legacy (ADR 0047)';
  END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE "automation_step_records" ADD COLUMN "http_status" integer NULL, ADD COLUMN "http_body" jsonb NULL, ADD COLUMN "http_dropped" character varying NULL;
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_http_body_check" CHECK ((http_body IS NULL) OR ((http_status IS NOT NULL) AND (http_dropped IS NULL))) NOT VALID;
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_http_dropped_check" CHECK ((http_dropped)::text = ANY ((ARRAY['not_json'::character varying, 'not_object'::character varying, 'too_large'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_skipped_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_skipped_check" CHECK ((((outcome)::text = 'done'::text) AND (skip_reason IS NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL) AND (awaited_event_id IS NULL) AND (awaited_action IS NULL) AND (awaited_properties IS NULL)) OR (((outcome)::text = 'skipped'::text) AND (skip_reason IS NOT NULL) AND (http_status IS NULL) AND (http_body IS NULL) AND (http_dropped IS NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL) AND (awaited_event_id IS NULL) AND (awaited_action IS NULL) AND (awaited_properties IS NULL)) OR (((outcome)::text = 'path'::text) AND (path_id IS NOT NULL) AND (http_status IS NULL) AND (http_body IS NULL) AND (http_dropped IS NULL) AND (skip_reason IS NULL))) NOT VALID;

-- +goose Down
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_skipped_check";
ALTER TABLE "automation_step_records" ADD CONSTRAINT "automation_step_records_skipped_check" CHECK ((((outcome)::text = 'done'::text) AND (skip_reason IS NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL) AND (awaited_event_id IS NULL) AND (awaited_action IS NULL) AND (awaited_properties IS NULL)) OR (((outcome)::text = 'skipped'::text) AND (skip_reason IS NOT NULL) AND (path_id IS NULL) AND (path_outcomes IS NULL) AND (awaited_event_id IS NULL) AND (awaited_action IS NULL) AND (awaited_properties IS NULL)) OR (((outcome)::text = 'path'::text) AND (path_id IS NOT NULL) AND (skip_reason IS NULL)));
ALTER TABLE "automation_step_records" DROP CONSTRAINT "automation_step_records_http_dropped_check", DROP CONSTRAINT "automation_step_records_http_body_check", DROP COLUMN "http_dropped", DROP COLUMN "http_body", DROP COLUMN "http_status";
