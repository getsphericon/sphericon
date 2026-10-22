-- +goose Up
-- A Usage row's finality becomes finalized_at (nil = Live usage, set = Usage snapshot), the moment
-- the period closed (ADR 0033, review rule 7). The `final` column is removed, so a Major is required.
-- A final row gets its computed_at, the moment its closing figure was computed. The table holds one
-- row per Workspace, period and metric, so the backfill is small work.
-- modify "usages" table
ALTER TABLE "usages" ADD COLUMN "finalized_at" timestamptz NULL;
UPDATE "usages" SET "finalized_at" = "computed_at" WHERE "final";
-- A final row no moment carries would lose its finality by the drop: refuse instead.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "usages"
    WHERE ("final" AND "finalized_at" IS NULL) OR (NOT "final" AND "finalized_at" IS NOT NULL)
  ) THEN
    RAISE EXCEPTION 'usages: a final state has no moment to carry it; refusing to drop "final"';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE "usages" DROP COLUMN "final";

-- +goose Down
-- reverse: modify "usages" table
ALTER TABLE "usages" ADD COLUMN "final" boolean NOT NULL DEFAULT false;
UPDATE "usages" SET "final" = ("finalized_at" IS NOT NULL);
ALTER TABLE "usages" DROP COLUMN "finalized_at";
