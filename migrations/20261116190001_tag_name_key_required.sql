-- +goose Up
-- Every Tag has its name's key once the Go migration TagNameKeysVersion has run, and a Workspace
-- has one Tag per key, so the key is required and unique in a Workspace. A database that ran the
-- first version of 20261112165107_tag_name_key has both already. tags holds a few rows per
-- Workspace, so the NOT NULL check and the index build scan little.
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "tags" ALTER COLUMN "name_key" SET NOT NULL;
-- squawk-ignore require-concurrent-index-creation
CREATE UNIQUE INDEX IF NOT EXISTS "tags_workspace_id_name_key" ON "tags" ("workspace_id", "name_key");

-- +goose Down
DROP INDEX "tags_workspace_id_name_key";
ALTER TABLE "tags" ALTER COLUMN "name_key" DROP NOT NULL;
