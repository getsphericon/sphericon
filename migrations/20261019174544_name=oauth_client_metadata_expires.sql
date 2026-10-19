-- +goose Up
-- modify "oauth_clients" table
ALTER TABLE "oauth_clients" ADD COLUMN "metadata_expires_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "oauth_clients" table
ALTER TABLE "oauth_clients" DROP COLUMN "metadata_expires_at";
