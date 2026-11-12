-- +goose Up
-- The key the APIs seal primary keys with (ADR 0131): 16 random bytes for the Instance's one row,
-- then made by the app (the default only fills that row).
-- squawk-ignore adding-field-with-default
ALTER TABLE "instance_state" ADD COLUMN "id_key" bytea NOT NULL DEFAULT uuid_send(gen_random_uuid());
ALTER TABLE "instance_state" ALTER COLUMN "id_key" DROP DEFAULT;

-- +goose Down
ALTER TABLE "instance_state" DROP COLUMN "id_key";
