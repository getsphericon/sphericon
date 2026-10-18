-- +goose Up
-- Destructive (ADR 0033, ADR 0037): the Workspace's ingest key goes, every Integration has its own.
-- The guard first checks that the keys it replaces have moved: an Integration with no key of its
-- own means the previous migration did not run, and dropping now would strand its hook.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "integrations" WHERE "ingest_key" IS NULL OR "ingest_key" = '') THEN
    RAISE EXCEPTION 'workspaces.ingest_key cannot be dropped: an Integration has no ingest key of its own; install the last release of Major 1, migrate, then upgrade again';
  END IF;
END
$$;
-- +goose StatementEnd
-- modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "ingest_key";
-- A new Major (2) whose floor is the last release of Major 1: nothing older runs on this schema.
UPDATE "schema_state" SET "major" = 2, "floor" = 'v0.1.2' WHERE "id" = 1;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "ingest_key" character varying NULL;
UPDATE "workspaces" SET "ingest_key" = 'omik_' || replace(gen_random_uuid()::text, '-', '');
ALTER TABLE "workspaces" ALTER COLUMN "ingest_key" SET NOT NULL;
CREATE UNIQUE INDEX "workspaces_ingest_key_key" ON "workspaces" ("ingest_key");
UPDATE "schema_state" SET "major" = 1, "floor" = 'v0.0.0' WHERE "id" = 1;
