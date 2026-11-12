-- +goose Up
-- modify "custom_fields" table
ALTER TABLE "custom_fields" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;

-- +goose Down
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" DROP COLUMN "revision";
-- reverse: modify "custom_fields" table
ALTER TABLE "custom_fields" DROP COLUMN "revision";
