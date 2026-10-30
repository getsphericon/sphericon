-- +goose NO TRANSACTION
-- +goose Up
-- modify "api_tokens" table: a Connection is an API token tied to the approving User's Membership (ADR 0061)
ALTER TABLE "api_tokens" ADD COLUMN "membership_id" bigint NULL;
ALTER TABLE "api_tokens" ADD CONSTRAINT "api_tokens_memberships_connections" FOREIGN KEY ("membership_id") REFERENCES "memberships" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "api_tokens" VALIDATE CONSTRAINT "api_tokens_memberships_connections";
-- create index "apitoken_membership_id" to table: "api_tokens"
CREATE INDEX CONCURRENTLY "apitoken_membership_id" ON "api_tokens" ("membership_id");

-- +goose Down
-- reverse: create index "apitoken_membership_id" to table: "api_tokens"
DROP INDEX CONCURRENTLY "apitoken_membership_id";
-- reverse: modify "api_tokens" table
ALTER TABLE "api_tokens" DROP CONSTRAINT "api_tokens_memberships_connections", DROP COLUMN "membership_id";
