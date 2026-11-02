-- +goose NO TRANSACTION
-- +goose Up
-- Destructive (ADR 0033, ADR 0081): the Integration moves from the Automation version to each of
-- its send steps. Every email step of a version that named an Integration gets it as its own
-- integrationId, in step order; a version that named none keeps steps without one (its Channel's
-- default, as before). The guard checks that move before the column goes, so no Integration is
-- stranded. ent's diff wrote the column drop; the rewrite and the guard are written by hand. A
-- version is one row per Automation edit, so the rewrite is a fast statement, not a river job.
UPDATE "automation_versions" v SET "steps" = (
  SELECT COALESCE(jsonb_agg(
    CASE WHEN t.step->>'type' = 'email'
      THEN t.step || jsonb_build_object('integrationId', v."integration_id")
      ELSE t.step
    END ORDER BY t.position), '[]'::jsonb)::text
  FROM jsonb_array_elements(v."steps"::jsonb) WITH ORDINALITY AS t(step, position)
)
WHERE v."integration_id" IS NOT NULL;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM "automation_versions" v, jsonb_array_elements(v."steps"::jsonb) AS t(step)
    WHERE v."integration_id" IS NOT NULL
      AND t.step->>'type' = 'email'
      AND (t.step->>'integrationId') IS DISTINCT FROM v."integration_id"::text
  ) THEN
    RAISE EXCEPTION 'automation_versions.integration_id cannot be dropped: an email step does not carry its version''s Integration yet (ADR 0081)';
  END IF;
END
$$;
-- +goose StatementEnd
-- modify "automation_versions" table
ALTER TABLE "automation_versions" DROP COLUMN "integration_id";

-- +goose Down
-- reverse: modify "automation_versions" table
ALTER TABLE "automation_versions" ADD COLUMN "integration_id" bigint NULL;
-- The version takes the Integration of its first email step that names one, and the steps lose
-- theirs: a version with one Integration per step comes back sending through the first of them.
UPDATE "automation_versions" v SET
  "integration_id" = (
    SELECT (t.step->>'integrationId')::bigint
    FROM jsonb_array_elements(v."steps"::jsonb) WITH ORDINALITY AS t(step, position)
    WHERE t.step->>'type' = 'email' AND t.step ? 'integrationId'
    ORDER BY t.position
    LIMIT 1
  ),
  "steps" = (
    SELECT COALESCE(jsonb_agg(t.step - 'integrationId' ORDER BY t.position), '[]'::jsonb)::text
    FROM jsonb_array_elements(v."steps"::jsonb) WITH ORDINALITY AS t(step, position)
  )
WHERE v."steps" LIKE '%integrationId%';
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_integrations_integration";
CREATE INDEX CONCURRENTLY "automationversion_integration_id" ON "automation_versions" ("integration_id");
