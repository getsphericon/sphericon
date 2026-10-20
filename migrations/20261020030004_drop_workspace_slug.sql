-- +goose Up
-- Destructive (ADR 0033, ADR 0043): the Workspace slug goes, the Workspace id is the one address.
-- The guard checks that the Workspace id is in place: the public_id column exists and every
-- Workspace carries a well-formed one (8 base62 characters, workspaceid.Pattern), the Go backfill
-- and the migration that made the column required having run. Dropping the slug first would leave a
-- Workspace with no address at all. Old /w/{slug} links and the slug argument of the command-line
-- tooling stop working by design.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'workspaces' AND column_name = 'public_id'
  ) THEN
    RAISE EXCEPTION 'workspaces.slug cannot be dropped: workspaces.public_id does not exist, so Workspaces would have no address; install the release that gives every Workspace an id, migrate, then upgrade again';
  END IF;
  IF EXISTS (SELECT 1 FROM "workspaces" WHERE "public_id" IS NULL OR "public_id" !~ '^[0-9A-Za-z]{8}$') THEN
    RAISE EXCEPTION 'workspaces.slug cannot be dropped: some Workspaces have no well-formed public_id, so they would have no address; install the release that gives every Workspace an id, migrate, then upgrade again';
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
