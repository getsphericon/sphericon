-- +goose NO TRANSACTION
-- +goose Up
-- Validates the credit check the previous migration added NOT VALID, and builds the index by which a
-- refusal of tracking finds the Orders its messages' clicks credited (ADR 0120).
ALTER TABLE "orders" VALIDATE CONSTRAINT "orders_credit_check";
CREATE INDEX CONCURRENTLY "order_workspace_id_credited_message_id" ON "orders" ("workspace_id", "credited_message_id");

-- +goose Down
DROP INDEX CONCURRENTLY "order_workspace_id_credited_message_id";
