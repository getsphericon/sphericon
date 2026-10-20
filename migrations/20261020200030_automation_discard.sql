-- +goose Up
-- modify "automations" table
ALTER TABLE "automations" ADD COLUMN "discarded_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP COLUMN "discarded_at";
