-- +goose Up
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD COLUMN "tracking_reason" character varying NULL;

-- +goose Down
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP COLUMN "tracking_reason";
