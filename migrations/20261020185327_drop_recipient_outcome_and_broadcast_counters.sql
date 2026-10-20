-- +goose Up
-- guard: a Broadcast recipient's outcome moves to its Outbound message (ADR 0015), so a recipient
-- decided before its message existed would lose it. Refuse rather than strand the data.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "broadcast_recipients" r
    WHERE r."status" <> 'pending'
      AND NOT EXISTS (
        SELECT 1 FROM "outbound_messages" m
        WHERE m."broadcast_recipient_id" = r."id" AND m."status" <> 'pending'
      )
  ) THEN
    RAISE EXCEPTION 'broadcast_recipients decided without an outbound message would lose their outcome: upgrade through the release that records one per recipient first';
  END IF;
END
$$;
-- +goose StatementEnd
-- modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" DROP COLUMN "status", DROP COLUMN "error", DROP COLUMN "sent_at", DROP COLUMN "outbound_message_id";
-- modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "recipients_total", DROP COLUMN "sent_count", DROP COLUMN "opened_count", DROP COLUMN "clicked_count", DROP COLUMN "unsubscribed_count", DROP COLUMN "failed_count", DROP COLUMN "skipped_count";

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "skipped_count" bigint NOT NULL DEFAULT 0, ADD COLUMN "failed_count" bigint NOT NULL DEFAULT 0, ADD COLUMN "unsubscribed_count" bigint NOT NULL DEFAULT 0, ADD COLUMN "clicked_count" bigint NOT NULL DEFAULT 0, ADD COLUMN "opened_count" bigint NOT NULL DEFAULT 0, ADD COLUMN "sent_count" bigint NOT NULL DEFAULT 0, ADD COLUMN "recipients_total" bigint NOT NULL DEFAULT 0;
-- reverse: modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" ADD COLUMN "outbound_message_id" bigint NULL, ADD COLUMN "sent_at" timestamptz NULL, ADD COLUMN "error" character varying NULL, ADD COLUMN "status" character varying NOT NULL DEFAULT 'pending';
