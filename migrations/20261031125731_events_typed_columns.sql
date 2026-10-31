-- +goose NO TRANSACTION
-- +goose Up
-- An Event's time and the facts of an email.* Event become typed columns, so reports state them
-- with ent's predicates instead of COALESCE(occurred_at, created_at) and keys read out of the JSON
-- properties. ent's diff wrote the columns and the NOT NULL; the copy of the values and the CHECK
-- that lets SET NOT NULL skip its own scan of the table are written by hand, outside a
-- transaction so the validation does not hold the lock the constraint took.
--
-- The copy below is data work over the whole table (the email.* Events are most of it), which
-- ADR 0033 sends to a river job. It runs here because the project has no production installs
-- yet (greenfield, AGENTS.md); a release after the first one would move it to a job.
-- modify "events" table
ALTER TABLE "events" ADD COLUMN "machine" boolean NOT NULL DEFAULT false, ADD COLUMN "captured" boolean NOT NULL DEFAULT false, ADD COLUMN "broadcast_id" bigint NULL, ADD COLUMN "automation_id" bigint NULL, ADD COLUMN "url" character varying NULL;
ALTER TABLE "events" ALTER COLUMN "occurred_at" SET DEFAULT CURRENT_TIMESTAMP;
-- The values the email.* Events carried in their properties, which keep them for segment
-- conditions. An id that is no JSON number is no Broadcast or Automation and stays NULL, so no
-- row can stop the copy halfway.
UPDATE "events"
SET "machine" = COALESCE("properties"->>'machine', '') = 'true',
    "captured" = COALESCE("properties"->>'captured', '') = 'true',
    "broadcast_id" = CASE WHEN jsonb_typeof("properties"->'broadcastId') = 'number' THEN ("properties"->>'broadcastId')::numeric::bigint END,
    "automation_id" = CASE WHEN jsonb_typeof("properties"->'automationId') = 'number' THEN ("properties"->>'automationId')::numeric::bigint END,
    "url" = "properties"->>'url'
WHERE "action" LIKE 'email.%';
-- The bus stamps every Event it stores with a time; a row written without one took its creation.
UPDATE "events" SET "occurred_at" = "created_at" WHERE "occurred_at" IS NULL;
ALTER TABLE "events" ADD CONSTRAINT "events_occurred_at_present" CHECK ("occurred_at" IS NOT NULL) NOT VALID;
ALTER TABLE "events" VALIDATE CONSTRAINT "events_occurred_at_present";
ALTER TABLE "events" ALTER COLUMN "occurred_at" SET NOT NULL;
ALTER TABLE "events" DROP CONSTRAINT "events_occurred_at_present";

-- +goose Down
-- reverse: modify "events" table
ALTER TABLE "events" DROP COLUMN "url", DROP COLUMN "automation_id", DROP COLUMN "broadcast_id", DROP COLUMN "captured", DROP COLUMN "machine", ALTER COLUMN "occurred_at" DROP NOT NULL, ALTER COLUMN "occurred_at" DROP DEFAULT;
