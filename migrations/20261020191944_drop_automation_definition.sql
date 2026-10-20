-- +goose Up
-- Destructive (ADR 0033, ADR 0049): an Automation's steps live in automation_versions, so the
-- definition column goes. The guard checks the data has moved: every Automation has a version and
-- every Enrollment stands on one, the river job backfill_automation_versions having run. Dropping
-- the column first would lose the steps of an Automation, and the steps an Enrollment stands on.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "automations" a
    WHERE NOT EXISTS (SELECT 1 FROM "automation_versions" v WHERE v."automation_id" = a."id")
  ) THEN
    RAISE EXCEPTION 'automations.definition cannot be dropped: some Automations have no version, so their steps would be lost; run the release that adds automation_versions until the job backfill_automation_versions has finished, then upgrade again';
  END IF;
  IF EXISTS (SELECT 1 FROM "automation_runs" WHERE "version_id" IS NULL) THEN
    RAISE EXCEPTION 'automations.definition cannot be dropped: some Enrollments stand on no version, so they would lose their steps; run the release that adds automation_versions until the job backfill_automation_versions has finished, then upgrade again';
  END IF;
END
$$;
-- +goose StatementEnd
-- modify "automations" table
ALTER TABLE "automations" DROP COLUMN "definition";

-- +goose Down
-- reverse: modify "automations" table; the steps come back from the latest version.
ALTER TABLE "automations" ADD COLUMN "definition" character varying NOT NULL DEFAULT '[]';
UPDATE "automations" a SET "definition" = v."steps"
FROM "automation_versions" v
WHERE v."automation_id" = a."id"
  AND v."version" = (SELECT MAX(l."version") FROM "automation_versions" l WHERE l."automation_id" = a."id");
