-- +goose Up
-- Each Tracking domain gets its ownership token (ADR 0173): added nullable, given a random one per
-- row, then required. tracking_domains holds a few rows per Workspace, so the NOT NULL check scans
-- little.
-- modify "tracking_domains" table
ALTER TABLE "tracking_domains" ADD COLUMN "ownership_token" character varying NULL;
UPDATE "tracking_domains" SET "ownership_token" = md5(random()::text || "id"::text) WHERE "ownership_token" IS NULL;
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "tracking_domains" ALTER COLUMN "ownership_token" SET NOT NULL;

-- +goose Down
-- reverse: modify "tracking_domains" table
ALTER TABLE "tracking_domains" DROP COLUMN "ownership_token";
