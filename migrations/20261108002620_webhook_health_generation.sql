-- +goose Up
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD COLUMN "health_generation" bigint NULL;

-- +goose Down
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" DROP COLUMN "health_generation";
