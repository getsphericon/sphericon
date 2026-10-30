-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "condition" character varying NULL;
-- An armed Broadcast (scheduled, or sending and still to re-plan) takes the copy of its Segment's
-- Condition it would have taken on leaving draft (ADR 0065); a draft, a finished one and one to all
-- contacts copy nothing.
UPDATE "broadcasts" SET "condition" = COALESCE("segments"."condition", '')
FROM "segments"
WHERE "segments"."id" = "broadcasts"."segment_id"
  AND "broadcasts"."status" IN ('scheduled', 'sending');

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "condition";
