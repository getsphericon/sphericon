-- +goose Up
-- modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "preset" character varying NULL, ADD COLUMN "params_encrypted" character varying NULL, ADD COLUMN "ingest_key" character varying NULL;
-- Every existing Integration gets its own hook key (ADR 0037); the column is only then required.
UPDATE "integrations" SET "ingest_key" = 'omik_' || replace(gen_random_uuid()::text, '-', '') WHERE "ingest_key" IS NULL;
ALTER TABLE "integrations" ALTER COLUMN "ingest_key" SET NOT NULL;
-- create index "integrations_ingest_key_key" to table: "integrations"
CREATE UNIQUE INDEX "integrations_ingest_key_key" ON "integrations" ("ingest_key");

-- +goose Down
-- reverse: create index "integrations_ingest_key_key" to table: "integrations"
DROP INDEX "integrations_ingest_key_key";
-- reverse: modify "integrations" table
ALTER TABLE "integrations" DROP COLUMN "ingest_key", DROP COLUMN "params_encrypted", DROP COLUMN "preset";
