-- +goose NO TRANSACTION
-- +goose Up
-- modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "discarded_at" timestamptz NULL;
-- swap index "integration_workspace_id_channel" to the partial form: an archived Integration holds no default
CREATE UNIQUE INDEX CONCURRENTLY "integration_workspace_id_channel_kept" ON "integrations" ("workspace_id", "channel") WHERE (is_default AND (discarded_at IS NULL));
DROP INDEX CONCURRENTLY "integration_workspace_id_channel";
ALTER INDEX "integration_workspace_id_channel_kept" RENAME TO "integration_workspace_id_channel";

-- +goose Down
-- reverse: swap index "integration_workspace_id_channel" back to is_default alone
CREATE UNIQUE INDEX CONCURRENTLY "integration_workspace_id_channel_all" ON "integrations" ("workspace_id", "channel") WHERE is_default;
DROP INDEX CONCURRENTLY "integration_workspace_id_channel";
ALTER INDEX "integration_workspace_id_channel_all" RENAME TO "integration_workspace_id_channel";
-- reverse: modify "integrations" table
ALTER TABLE "integrations" DROP COLUMN "discarded_at";
