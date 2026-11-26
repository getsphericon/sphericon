-- +goose Up
-- A Broadcast recipient keeps since when it waits (ADR 0178), beside why and until when. ent's diff
-- wrote the column and the check; the check is added NOT VALID, and the next migration
-- (20261126224634) validates it.
ALTER TABLE "broadcast_recipients" ADD COLUMN "deferred_since" timestamptz NULL;
ALTER TABLE "broadcast_recipients" ADD CONSTRAINT "broadcast_recipients_deferred_since_check" CHECK ((deferred_since IS NULL) OR (deferral_cause IS NOT NULL)) NOT VALID;

-- +goose Down
ALTER TABLE "broadcast_recipients" DROP CONSTRAINT "broadcast_recipients_deferred_since_check", DROP COLUMN "deferred_since";
