-- +goose Up
-- Destructive (ADR 0033, ADR 0178): an Outbound message's reason splits into its Skipped reason, one
-- of a closed list, and its failure text. A skipped message keeps its reason in skipped_reason and a
-- failed one its text in failure; reason goes. The checks are validated by the next migration.
--
-- The copy below is data work over the whole table, which ADR 0033 sends to a river job. It runs here
-- because the project has no production installs yet (greenfield, AGENTS.md).
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "outbound_messages"
    WHERE ("status" = 'skipped' AND ("reason" IS NULL OR "reason" NOT IN ('suppressed', 'unsubscribed_everything', 'unsubscribed_source', 'unconfirmed', 'no_destination', 'contact_erased', 'no_local_time', 'frequency_cap')))
       OR ("status" IN ('pending', 'sent') AND "reason" IS NOT NULL)
  ) THEN
    RAISE EXCEPTION 'outbound_messages.reason cannot be split: a skipped message has a reason outside the Skipped reasons, or a pending or sent message has one; set them by hand (ADR 0178), then migrate again';
  END IF;
END
$$;
-- +goose StatementEnd
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "skipped_reason" character varying NULL, ADD COLUMN "failure" character varying NULL;
UPDATE "outbound_messages" SET "skipped_reason" = "reason" WHERE "status" = 'skipped';
UPDATE "outbound_messages" SET "failure" = "reason" WHERE "status" = 'failed';
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_skipped_reason_check" CHECK ((skipped_reason)::text = ANY ((ARRAY['suppressed'::character varying, 'unsubscribed_everything'::character varying, 'unsubscribed_source'::character varying, 'unconfirmed'::character varying, 'no_destination'::character varying, 'contact_erased'::character varying, 'no_local_time'::character varying, 'frequency_cap'::character varying])::text[])) NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_outcome_check" CHECK ((((status)::text = 'pending'::text) AND (skipped_reason IS NULL) AND (failure IS NULL)) OR (((status)::text = 'sent'::text) AND (skipped_reason IS NULL) AND (failure IS NULL)) OR (((status)::text = 'skipped'::text) AND (skipped_reason IS NOT NULL) AND (failure IS NULL)) OR (((status)::text = 'failed'::text) AND (skipped_reason IS NULL))) NOT VALID;
ALTER TABLE "outbound_messages" DROP COLUMN "reason";

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "reason" character varying NULL;
UPDATE "outbound_messages" SET "reason" = COALESCE("skipped_reason", "failure");
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_outcome_check", DROP CONSTRAINT "outbound_messages_skipped_reason_check", DROP COLUMN "failure", DROP COLUMN "skipped_reason";
