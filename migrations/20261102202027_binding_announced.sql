-- +goose Up
-- modify "bindings" table
ALTER TABLE "bindings" ADD COLUMN "announced" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "bindings" table
ALTER TABLE "bindings" DROP COLUMN "announced";
