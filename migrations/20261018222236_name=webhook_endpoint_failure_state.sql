-- +goose Up
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD COLUMN "consecutive_failures" bigint NULL, ADD COLUMN "failing_since" timestamptz NULL, ADD COLUMN "last_error_kind" character varying NULL, ADD COLUMN "last_error_status" bigint NULL, ADD COLUMN "last_failure_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" DROP COLUMN "last_failure_at", DROP COLUMN "last_error_status", DROP COLUMN "last_error_kind", DROP COLUMN "failing_since", DROP COLUMN "consecutive_failures";
