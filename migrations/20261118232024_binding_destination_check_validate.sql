-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID.
ALTER TABLE "bindings" VALIDATE CONSTRAINT "bindings_destination_check";

-- +goose Down
