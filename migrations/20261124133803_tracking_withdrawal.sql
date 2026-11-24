-- +goose NO TRANSACTION
-- +goose Up
-- The index by which a refusal of tracking finds its Destination's Outbound messages, in id order, a
-- batch at a time (ADR 0120).
CREATE INDEX CONCURRENTLY "outboundmessage_workspace_id_destination_id" ON "outbound_messages" ("workspace_id", "destination", "id");

-- +goose Down
DROP INDEX CONCURRENTLY "outboundmessage_workspace_id_destination_id";
