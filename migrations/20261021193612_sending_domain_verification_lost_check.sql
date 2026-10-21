-- +goose NO TRANSACTION
-- +goose Up
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD CONSTRAINT "sending_domains_verification_lost_check" CHECK ((verification_lost_at IS NULL) OR (verified_at IS NOT NULL)) NOT VALID;
ALTER TABLE "sending_domains" VALIDATE CONSTRAINT "sending_domains_verification_lost_check";

-- +goose Down
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP CONSTRAINT "sending_domains_verification_lost_check";
