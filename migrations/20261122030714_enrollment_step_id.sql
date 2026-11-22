-- +goose Up
-- Destructive (ADR 0033, ADR 0122): an Enrollment names the Step it stands on by the Step's id, not
-- its position, so a Step inserted above it in a later version moves nothing. It sorts after the
-- Go migration that gives every stored Step an id (20261122020713), and rides the Major that one
-- starts. ent's diff wrote the column swap; the copy, the guard and the jobs are written by hand.
--
-- Each Enrollment gets the id of the Step at its position in the version it began on. One that
-- stood past the last Step gets none: it stays as it is, and its queued job, which names no Step,
-- completes it as the job of its position did.
ALTER TABLE "automation_runs" ADD COLUMN "current_step_id" character varying NULL;
UPDATE "automation_runs" r
SET "current_step_id" = v."steps" -> 'steps' -> r."current_step"::int ->> 'id'
FROM "automation_versions" v
WHERE v."id" = r."version_id";
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM "automation_runs" r
    JOIN "automation_versions" v ON v."id" = r."version_id"
    WHERE r."current_step_id" IS NULL
      AND r."current_step" < jsonb_array_length(v."steps" -> 'steps')
  ) THEN
    RAISE EXCEPTION 'automation_runs.current_step cannot be dropped: an Enrollment on a Step does not name its id yet (ADR 0122)';
  END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE "automation_runs" DROP COLUMN "current_step";

-- The step jobs and HTTP-step jobs still to run name the id of the Step at the position they
-- named ("" past the last Step, or for an Enrollment gone with its Contact), the first job of an
-- Enrollment, which named none, the first Step. The release is stopped for this Major, so a job
-- left running by it is rewritten too: the next release retries it.
UPDATE "river_job" j
SET "args" = (j."args" - 'step') || jsonb_build_object('step_id', COALESCE((
  SELECT v."steps" -> 'steps' -> COALESCE((j."args" ->> 'step')::int, 0) ->> 'id'
  FROM "automation_runs" r
  JOIN "automation_versions" v ON v."id" = r."version_id"
  WHERE r."id" = (j."args" ->> 'run_id')::bigint
), ''))
WHERE j."kind" IN ('automation_run_step', 'automation_http_step')
  AND j."state" NOT IN ('completed', 'cancelled', 'discarded')
  AND NOT j."args" ? 'step_id';
-- An HTTP-step job is unique by its args: river keeps the key it derived from the old args, so the
-- key is derived again from the new ones, as river derives it (the SHA-256 of the kind and the
-- args' keys sorted, compact), and a step job delivered again still finds the queued call.
UPDATE "river_job" j
SET "unique_key" = sha256(convert_to('&kind=' || j."kind" || '&args=' || (
  SELECT '{' || string_agg(to_json(a.key)::text || ':' || a.value::text, ',' ORDER BY a.key COLLATE "C") || '}'
  FROM jsonb_each(j."args") AS a(key, value)
), 'UTF8'))
WHERE j."kind" = 'automation_http_step'
  AND j."unique_key" IS NOT NULL
  AND j."state" NOT IN ('completed', 'cancelled', 'discarded');

-- +goose Down
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" ADD COLUMN "current_step" bigint NOT NULL DEFAULT 0;
-- An Enrollment takes the position of its Step, past the last Step when it names none.
UPDATE "automation_runs" r
SET "current_step" = COALESCE((
  SELECT t.position - 1
  FROM jsonb_array_elements(v."steps" -> 'steps') WITH ORDINALITY AS t(step, position)
  WHERE t.step ->> 'id' = r."current_step_id"
), jsonb_array_length(v."steps" -> 'steps'))
FROM "automation_versions" v
WHERE v."id" = r."version_id";
ALTER TABLE "automation_runs" DROP COLUMN "current_step_id";
UPDATE "river_job" j
SET "args" = (j."args" - 'step_id') || jsonb_build_object('step', COALESCE((
  SELECT t.position - 1
  FROM "automation_runs" r
  JOIN "automation_versions" v ON v."id" = r."version_id",
  jsonb_array_elements(v."steps" -> 'steps') WITH ORDINALITY AS t(step, position)
  WHERE r."id" = (j."args" ->> 'run_id')::bigint AND t.step ->> 'id' = j."args" ->> 'step_id'
), (
  SELECT jsonb_array_length(v."steps" -> 'steps')
  FROM "automation_runs" r
  JOIN "automation_versions" v ON v."id" = r."version_id"
  WHERE r."id" = (j."args" ->> 'run_id')::bigint
), 0))
WHERE j."kind" IN ('automation_run_step', 'automation_http_step')
  AND j."state" NOT IN ('completed', 'cancelled', 'discarded')
  AND j."args" ? 'step_id';
