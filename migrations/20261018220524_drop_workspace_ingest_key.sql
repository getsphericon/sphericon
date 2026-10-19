-- +goose Up
-- Destructive (ADR 0033, ADR 0037): the Workspace's ingest key goes, every Integration has its own.
-- The guard checks that the previous migration ran, which is what made the replacement keys
-- exist: integrations.ingest_key is there and NOT NULL (that migration backfills every row,
-- then sets NOT NULL, so a row with no key of its own cannot exist once it is). Dropping
-- without it would leave a Workspace with no hook key at all. The Workspace's old URLs stop
-- working by design: an Integration's hook URL is shown in the app and in the API.
-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'integrations'
      AND column_name = 'ingest_key' AND is_nullable = 'NO'
  ) THEN
    RAISE EXCEPTION 'workspaces.ingest_key cannot be dropped: integrations.ingest_key is missing or nullable, so the Integrations have no hook keys of their own; install the last release of Major 1, migrate, then upgrade again';
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
