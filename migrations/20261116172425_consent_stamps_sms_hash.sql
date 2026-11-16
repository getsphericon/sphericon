-- +goose Up
-- An sms consent stamp is stored by the number's hash, never the number (ADR 0154, ADR 0021). A
-- stamp stored by the number before this is rekeyed to the hash (lowercase hex SHA-256, as
-- phone.Hash), and dropped where the hash has one already: the table is days old and small. The
-- check is validated by the next migration.
DELETE FROM "consent_stamps" AS plain USING "consent_stamps" AS hashed
WHERE plain.channel = 'sms' AND hashed.channel = 'sms'
  AND plain.destination !~ '^[0-9a-f]{64}$'
  AND hashed.workspace_id = plain.workspace_id AND hashed.sending_source = plain.sending_source
  AND hashed.destination = encode(sha256(convert_to(plain.destination, 'UTF8')), 'hex');
UPDATE "consent_stamps" SET destination = encode(sha256(convert_to(destination, 'UTF8')), 'hex')
WHERE channel = 'sms' AND destination !~ '^[0-9a-f]{64}$';
-- modify "consent_stamps" table
ALTER TABLE "consent_stamps" ADD CONSTRAINT "consent_stamps_sms_hash_check" CHECK (((channel)::text <> 'sms'::text) OR ((destination)::text ~ '^[0-9a-f]{64}$'::text)) NOT VALID;

-- +goose Down
-- reverse: modify "consent_stamps" table
ALTER TABLE "consent_stamps" DROP CONSTRAINT "consent_stamps_sms_hash_check";
