-- +goose Up
-- A Tag is matched by its name's key, trimmed and lower-cased (tagname.Key), so "VIP" and "vip" are
-- one Tag; the key is unique in a Workspace in place of the name as written.
--
-- Edited on main, against the rule that a migration on main is never edited: its first version
-- added the column NOT NULL without a value, so it failed on every database holding a Tag and
-- passed only where tags was empty. A database that ran it has nothing this version changes; one
-- that stopped before it runs this one. A later migration cannot help, because goose stops here.
--
-- The column is added nullable. The Go migration TagNameKeysVersion fills it with tagname.Key and
-- merges the Tags whose names share a key, since lower() follows the database's locale and leaves
-- "ТЕГ" as it is under "C"; 20261116190001_tag_name_key_required then requires it and builds the
-- unique index.
-- squawk-ignore require-concurrent-index-deletion
DROP INDEX "tags_workspace_id_name";
ALTER TABLE "tags" ADD COLUMN "name_key" character varying NULL;

-- +goose Down
ALTER TABLE "tags" DROP COLUMN "name_key";
CREATE UNIQUE INDEX "tags_workspace_id_name" ON "tags" ("workspace_id", "name");
