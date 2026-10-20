-- +goose Up
-- Destructive (ADR 0033, ADR 0043): the Workspace slug goes, the Workspace id is the one address.
-- The guard checks that the Workspace id exists for every Workspace (the Go backfill and the
-- migration that made the column required have run): dropping the slug first would leave a
-- Workspace with no address at all. Old /w/{slug} links and the slug argument of the command-line
-- tooling stop working by design.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "workspaces" WHERE "public_id" IS NULL OR "public_id" = '') THEN
    RAISE EXCEPTION 'workspaces.slug cannot be dropped: some Workspaces have no public_id, so they would have no address; install the release that gives every Workspace an id, migrate, then upgrade again';
  END IF;
END
$$;
-- +goose StatementEnd
-- modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "slug";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "slug" character varying NULL;
UPDATE "workspaces" SET "slug" = lower("public_id");
ALTER TABLE "workspaces" ALTER COLUMN "slug" SET NOT NULL;
CREATE UNIQUE INDEX "workspaces_slug_key" ON "workspaces" ("slug");
