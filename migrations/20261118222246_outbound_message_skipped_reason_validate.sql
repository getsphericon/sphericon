-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID.
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_skipped_reason_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_outcome_check";

-- +goose Down
