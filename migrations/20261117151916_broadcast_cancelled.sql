-- +goose Up
-- modify "broadcasts" table: a sending Broadcast may be cancelled (ADR 0150), a final status that
-- keeps when it was cancelled. ent's diff does not see an enum's check widen, so the status check is
-- written by hand. The checks are validated by the next migration.
ALTER TABLE "broadcasts" ADD COLUMN "cancelled_at" timestamptz NULL;
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_status_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'scheduled'::character varying, 'sending'::character varying, 'sent'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_cancelled_at_check" CHECK (((status)::text <> 'cancelled'::text) OR (cancelled_at IS NOT NULL)) NOT VALID;

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_cancelled_at_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_status_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'scheduled'::character varying, 'sending'::character varying, 'sent'::character varying, 'failed'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" DROP COLUMN "cancelled_at";
