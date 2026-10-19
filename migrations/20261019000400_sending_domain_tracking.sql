-- +goose Up
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD COLUMN "tracking_state" character varying NOT NULL DEFAULT 'pending', ADD COLUMN "tracking_checked_at" timestamptz NULL, ADD COLUMN "tracking_state_changed_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP COLUMN "tracking_state_changed_at", DROP COLUMN "tracking_checked_at", DROP COLUMN "tracking_state";
