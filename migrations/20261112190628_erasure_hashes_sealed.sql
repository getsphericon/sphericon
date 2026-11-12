-- +goose Up
-- An Erasure's hashes are sealed with the Instance's cipher (ADR 0045); the clear columns go.
-- squawk-ignore ban-drop-column
ALTER TABLE "erasures" DROP COLUMN "email_sha256", DROP COLUMN "phone_sha256", ADD COLUMN "email_hash_encrypted" character varying NULL, ADD COLUMN "phone_hash_encrypted" character varying NULL;

-- +goose Down
-- reverse: modify "erasures" table
ALTER TABLE "erasures" DROP COLUMN "phone_hash_encrypted", DROP COLUMN "email_hash_encrypted", ADD COLUMN "phone_sha256" character varying NULL, ADD COLUMN "email_sha256" character varying NULL;
