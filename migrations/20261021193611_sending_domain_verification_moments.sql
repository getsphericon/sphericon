-- +goose Up
-- Sending domain verification becomes two moments (verified_at, verification_lost_at) instead of
-- the boolean `verified` (ADR 0010, ADR 0033). The column is removed, so a Major is required.
-- A domain that was verified once and is not now (a lost verification, or a changed selector) must
-- stay unverified: it gets a loss moment before the flag is dropped, or the drop would turn it
-- verified. The table holds one row per Sending domain, so the backfill is small work.
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD COLUMN "verification_lost_at" timestamptz NULL;
UPDATE "sending_domains" SET "verification_lost_at" = COALESCE("last_checked_at", "updated_at") WHERE NOT "verified" AND "verified_at" IS NOT NULL;
-- A state no moment carries would be lost by the drop: refuse instead.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "sending_domains"
    WHERE ("verified" AND "verified_at" IS NULL)
       OR (NOT "verified" AND "verified_at" IS NOT NULL AND "verification_lost_at" IS NULL)
  ) THEN
    RAISE EXCEPTION 'sending_domains: a verified state has no moment to carry it; refusing to drop "verified"';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE "sending_domains" DROP CONSTRAINT "sending_domains_verified_check";
ALTER TABLE "sending_domains" DROP COLUMN "verified";

-- +goose Down
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" ADD COLUMN "verified" boolean NOT NULL DEFAULT false;
UPDATE "sending_domains" SET "verified" = ("verified_at" IS NOT NULL AND "verification_lost_at" IS NULL);
ALTER TABLE "sending_domains" ADD CONSTRAINT "sending_domains_verified_check" CHECK ((NOT verified) OR (verified_at IS NOT NULL));
ALTER TABLE "sending_domains" DROP COLUMN "verification_lost_at";
