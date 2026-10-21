-- +goose NO TRANSACTION
-- +goose Up
-- modify "suppressions" table
ALTER TABLE "suppressions" ADD COLUMN "lifted_at" timestamptz NULL;
-- swap index "suppressions_workspace_id_channel_destination" to a partial one (ADR 0049): the
-- partial index is built beside the old one under a temporary name, so a destination is never
-- unprotected, then takes the old name
CREATE UNIQUE INDEX CONCURRENTLY "suppressions_active_tmp" ON "suppressions" ("workspace_id", "channel", "destination") WHERE (lifted_at IS NULL);
DROP INDEX CONCURRENTLY "suppressions_workspace_id_channel_destination";
ALTER INDEX "suppressions_active_tmp" RENAME TO "suppressions_workspace_id_channel_destination";

-- +goose Down
-- reverse: swap the index back to the full one; lifted rows are removed first, or two rows of
-- one destination would refuse it
DELETE FROM "suppressions" WHERE "lifted_at" IS NOT NULL;
CREATE UNIQUE INDEX CONCURRENTLY "suppressions_full_tmp" ON "suppressions" ("workspace_id", "channel", "destination");
DROP INDEX CONCURRENTLY "suppressions_workspace_id_channel_destination";
ALTER INDEX "suppressions_full_tmp" RENAME TO "suppressions_workspace_id_channel_destination";
-- reverse: modify "suppressions" table
ALTER TABLE "suppressions" DROP COLUMN "lifted_at";
