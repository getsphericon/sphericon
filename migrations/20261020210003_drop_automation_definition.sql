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
    RAISE EXCEPTION 'automations.definition cannot be dropped: some Automations have no version, so their steps would be lost; the river job backfill_automation_versions has not moved the data yet; let it finish, then migrate again (on a development database, `mise run db:reset` rebuilds it)';
  END IF;
  IF EXISTS (SELECT 1 FROM "automation_runs" WHERE "version_id" IS NULL) THEN
    RAISE EXCEPTION 'automations.definition cannot be dropped: some Enrollments stand on no version, so they would lose their steps; the river job backfill_automation_versions has not moved the data yet; let it finish, then migrate again (on a development database, `mise run db:reset` rebuilds it)';
  END IF;
END
$$;
-- +goose StatementEnd
-- The guard above proved every Enrollment has a version, so the constraints the first migration left
-- NOT VALID can be finished here (the backfill job does the same when it runs first): validate, make
-- the column NOT NULL (no scan, the validated check proves it) and drop the check, so the database
-- has the shape of the ent schema also when the job never had a row to move.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'automation_runs_automation_versions_runs' AND NOT convalidated) THEN
    ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_automation_versions_runs";
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'automation_runs_version_id_required') THEN
    ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_version_id_required";
    ALTER TABLE "automation_runs" ALTER COLUMN "version_id" SET NOT NULL;
    ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_version_id_required";
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
