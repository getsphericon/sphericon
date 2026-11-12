-- +goose NO TRANSACTION
-- +goose Up
-- Backs the prune of receipts by age, which ends a receipt with the Event retention window.
CREATE INDEX CONCURRENTLY "eventreceipt_created_at" ON "event_receipts" ("created_at");

-- +goose Down
DROP INDEX CONCURRENTLY "eventreceipt_created_at";
