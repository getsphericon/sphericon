-- +goose NO TRANSACTION
-- +goose Up
-- modify "api_tokens" table: a Connection names the OAuth client its User approved, and a User's token that names none is a Personal token (ADR 0061)
ALTER TABLE "api_tokens" ADD COLUMN "oauth_client_id" bigint NULL;
ALTER TABLE "api_tokens" ADD CONSTRAINT "api_tokens_oauth_clients_connections" FOREIGN KEY ("oauth_client_id") REFERENCES "oauth_clients" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "api_tokens" VALIDATE CONSTRAINT "api_tokens_oauth_clients_connections";
ALTER TABLE "api_tokens" ADD CONSTRAINT "api_tokens_oauth_client_check" CHECK ((oauth_client_id IS NULL) OR (membership_id IS NOT NULL)) NOT VALID;
ALTER TABLE "api_tokens" VALIDATE CONSTRAINT "api_tokens_oauth_client_check";
-- create index "apitoken_oauth_client_id" to table: "api_tokens"
CREATE INDEX CONCURRENTLY "apitoken_oauth_client_id" ON "api_tokens" ("oauth_client_id");

-- +goose Down
-- reverse: create index "apitoken_oauth_client_id" to table: "api_tokens"
DROP INDEX CONCURRENTLY "apitoken_oauth_client_id";
-- reverse: modify "api_tokens" table
ALTER TABLE "api_tokens" DROP CONSTRAINT "api_tokens_oauth_clients_connections", DROP COLUMN "oauth_client_id", DROP CONSTRAINT "api_tokens_oauth_client_check";
