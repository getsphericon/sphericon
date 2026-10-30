-- +goose Up
-- The data of the previous migration (ADR 0065), written by hand: ent's diff writes no data. It is
-- one pass over the Automations and their versions, a few rows per Automation, so it runs here
-- rather than in a river job (ADR 0033): the breaking ticket of spec #767 checks it moved before
-- it drops the Automation's own Trigger and Integration.
--
-- Every version gets the Automation's Trigger and Integration: the older versions ran with them.
UPDATE "automation_versions" AS v
SET "trigger_event" = a."trigger_event", "integration_id" = a."integration_id"
FROM "automations" AS a
WHERE a."id" = v."automation_id";

-- An active Automation, and an inactive one whose newest version has an Enrollment on it, did
-- what its newest version says: that version is live, and there is no draft.
WITH "newest" AS (
  SELECT DISTINCT ON ("automation_id") "automation_id", "id"
  FROM "automation_versions"
  ORDER BY "automation_id", "version" DESC
)
UPDATE "automations" AS a
SET "live_version_id" = n."id"
FROM "newest" AS n
WHERE n."automation_id" = a."id"
  AND (a."status" = 'active' OR EXISTS (SELECT 1 FROM "automation_runs" AS r WHERE r."version_id" = n."id"));

-- Any other Automation: a new version was written only over one with Enrollments, so the
-- second-newest is what ran and is live (none when there is one version), and the newest is
-- the draft. Contact erasure deletes Enrollments, so this reads the structure, not the runs.
WITH "ranked" AS (
  SELECT "automation_id", "id", row_number() OVER (PARTITION BY "automation_id" ORDER BY "version" DESC) AS "rank"
  FROM "automation_versions"
)
UPDATE "automations" AS a
SET "draft_version_id" = d."id", "live_version_id" = l."id"
FROM "ranked" AS d
LEFT JOIN "ranked" AS l ON l."automation_id" = d."automation_id" AND l."rank" = 2
WHERE d."automation_id" = a."id" AND d."rank" = 1 AND a."live_version_id" IS NULL;

-- +goose Down
-- The Automation's own Trigger and Integration come back from its latest version, the draft
-- when there is one, as before ADR 0065.
UPDATE "automations" AS a
SET "trigger_event" = v."trigger_event", "integration_id" = v."integration_id"
FROM "automation_versions" AS v
WHERE v."id" = COALESCE(a."draft_version_id", a."live_version_id");
UPDATE "automations" SET "live_version_id" = NULL, "draft_version_id" = NULL;
UPDATE "automation_versions" SET "trigger_event" = NULL, "integration_id" = NULL;
