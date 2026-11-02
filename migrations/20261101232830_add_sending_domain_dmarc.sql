-- +goose Up
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD COLUMN "dmarc_checked_at" timestamptz NULL, ADD COLUMN "dmarc_found_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP COLUMN "dmarc_found_at", DROP COLUMN "dmarc_checked_at";
