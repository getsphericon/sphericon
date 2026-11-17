-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID.
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_status_check";
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_cancelled_at_check";

-- +goose Down
