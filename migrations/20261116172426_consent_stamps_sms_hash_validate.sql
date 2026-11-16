-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID.
ALTER TABLE "consent_stamps" VALIDATE CONSTRAINT "consent_stamps_sms_hash_check";

-- +goose Down
