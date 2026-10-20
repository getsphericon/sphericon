-- +goose Up
-- The Go migration 20261020004439 (internal/db, workspacePublicIDs) gave every existing Workspace its
-- id between the two files; the column is only then required and unique (ADR 0043).
-- modify "workspaces" table
ALTER TABLE "workspaces" ALTER COLUMN "public_id" SET NOT NULL;
-- create index "workspaces_public_id_key" to table: "workspaces"
CREATE UNIQUE INDEX "workspaces_public_id_key" ON "workspaces" ("public_id");

-- +goose Down
-- reverse: create index "workspaces_public_id_key" to table: "workspaces"
DROP INDEX "workspaces_public_id_key";
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" ALTER COLUMN "public_id" DROP NOT NULL;
