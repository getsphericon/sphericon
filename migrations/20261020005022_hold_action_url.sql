-- +goose Up
-- modify "holds" table
ALTER TABLE "holds" ADD COLUMN "action_url" character varying NULL;

-- +goose Down
-- reverse: modify "holds" table
ALTER TABLE "holds" DROP COLUMN "action_url";
