-- +goose Up
-- modify "oauth_codes" table: an authorization code names the approving Membership (ADR 0061).
-- A code lives five minutes and one issued before the upgrade names none, so the pending ones are
-- dropped (the client asks again) under a lock, in this one transaction: the column is then added
-- to an empty table that nothing can write meanwhile, which needs no default and no table scan.
LOCK TABLE "oauth_codes" IN ACCESS EXCLUSIVE MODE;
DELETE FROM "oauth_codes";
-- squawk-ignore adding-required-field
ALTER TABLE "oauth_codes" ADD COLUMN "membership_id" bigint NOT NULL;
-- squawk-ignore adding-foreign-key-constraint, constraint-missing-not-valid
ALTER TABLE "oauth_codes" ADD CONSTRAINT "oauth_codes_memberships_oauth_codes" FOREIGN KEY ("membership_id") REFERENCES "memberships" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- create index "oauthcode_membership_id" to table: "oauth_codes"
-- squawk-ignore require-concurrent-index-creation
CREATE INDEX "oauthcode_membership_id" ON "oauth_codes" ("membership_id");

-- +goose Down
-- reverse: create index "oauthcode_membership_id" to table: "oauth_codes"
DROP INDEX "oauthcode_membership_id";
-- reverse: modify "oauth_codes" table
ALTER TABLE "oauth_codes" DROP CONSTRAINT "oauth_codes_memberships_oauth_codes", DROP COLUMN "membership_id";
