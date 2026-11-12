-- +goose Up
-- A Tag is matched by its name's key, trimmed and lower-cased (tagname.Key), so "VIP" and "vip" are
-- one Tag; the key is unique in a Workspace in place of the name as written.
-- squawk-ignore require-concurrent-index-deletion
DROP INDEX "tags_workspace_id_name";
-- squawk-ignore adding-required-field
ALTER TABLE "tags" ADD COLUMN "name_key" character varying NOT NULL;
-- squawk-ignore require-concurrent-index-creation
CREATE UNIQUE INDEX "tags_workspace_id_name_key" ON "tags" ("workspace_id", "name_key");

-- +goose Down
DROP INDEX "tags_workspace_id_name_key";
ALTER TABLE "tags" DROP COLUMN "name_key";
CREATE UNIQUE INDEX "tags_workspace_id_name" ON "tags" ("workspace_id", "name");
