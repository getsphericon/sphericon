-- +goose Up
-- modify "users" table
ALTER TABLE "users" ADD COLUMN "second_factor_generation" bigint NULL;

-- +goose Down
-- reverse: modify "users" table
ALTER TABLE "users" DROP COLUMN "second_factor_generation";
