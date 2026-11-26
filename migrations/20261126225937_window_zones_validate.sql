-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID: how a Sending window judged a message
-- is one of its four values (ADR 0134).
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_window_zones_check";

-- +goose Down
