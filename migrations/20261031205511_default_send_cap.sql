-- +goose Up
-- modify "instance_state" table
ALTER TABLE "instance_state" ADD COLUMN "default_send_cap" bigint NULL;

-- +goose Down
-- reverse: modify "instance_state" table
ALTER TABLE "instance_state" DROP COLUMN "default_send_cap";
