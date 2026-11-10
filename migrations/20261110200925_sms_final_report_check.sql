-- +goose NO TRANSACTION
-- +goose Up
-- A message's final report is delivered or undelivered, never both (ADR 0079). The one writer of
-- undelivered_at has always stamped it only while both moments were null, so the rows already hold
-- the rule; the CHECK is added NOT VALID and validated after, so the table is not locked while it
-- is checked.
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_final_report_check" CHECK ((delivered_at IS NULL) OR (undelivered_at IS NULL)) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_final_report_check";

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_final_report_check";
