-- +goose NO TRANSACTION
-- +goose Up
-- The index by which a refusal of one of an Outbound message's several Destinations reaches it
-- (ADR 0120).
CREATE INDEX CONCURRENTLY "outboundmessage_destinations" ON "outbound_messages" USING gin ("destinations") WHERE (destinations IS NOT NULL);

-- +goose Down
DROP INDEX CONCURRENTLY "outboundmessage_destinations";
