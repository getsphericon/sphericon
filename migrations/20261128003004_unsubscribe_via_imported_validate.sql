-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID: how a person stopped is one of its
-- seven ways, `imported` among them (ADR 0138).
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_via_check";

-- +goose Down
