-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "suspended_by_kind" character varying NULL, ADD COLUMN "suspended_by_id" character varying NULL;
-- migrate the free-form actor of ADR 0007 to the structured one of ADR 0026: "system" and "cli" are
-- kinds, any other value was an Operator id
UPDATE "workspaces" SET "suspended_by_kind" = CASE WHEN "suspended_by" IN ('system', 'cli') THEN "suspended_by" ELSE 'operator' END,
  "suspended_by_id" = CASE WHEN "suspended_by" IN ('system', 'cli') THEN NULL ELSE "suspended_by" END
WHERE "suspended_by" IS NOT NULL;
ALTER TABLE "workspaces" DROP COLUMN "suspended_by";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "suspended_by" character varying NULL;
UPDATE "workspaces" SET "suspended_by" = CASE WHEN "suspended_by_kind" = 'operator' THEN "suspended_by_id" ELSE "suspended_by_kind" END
WHERE "suspended_by_kind" IS NOT NULL;
ALTER TABLE "workspaces" DROP COLUMN "suspended_by_id", DROP COLUMN "suspended_by_kind";
