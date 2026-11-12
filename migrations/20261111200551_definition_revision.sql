-- +goose Up
-- modify "automations" table
ALTER TABLE "automations" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;
-- modify "rules" table
ALTER TABLE "rules" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;

-- +goose Down
-- reverse: modify "rules" table
ALTER TABLE "rules" DROP COLUMN "revision";
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP COLUMN "revision";
