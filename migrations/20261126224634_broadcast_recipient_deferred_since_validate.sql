-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID: a recipient waits since a moment only
-- beside the cause of its wait (ADR 0178).
ALTER TABLE "broadcast_recipients" VALIDATE CONSTRAINT "broadcast_recipients_deferred_since_check";

-- +goose Down
