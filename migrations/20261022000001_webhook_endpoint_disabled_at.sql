-- +goose Up
-- A Webhook endpoint's enabled state becomes disabled_at (nil = enabled) with disabled_reason
-- saying why (ADR 0033, review rule 7). The `enabled` column is removed, so a Major is required.
-- An endpoint an owner turned off had no reason; it gets "owner" so the reason is always kept.
-- A disabled endpoint gets the moment its last failure was recorded (the likeliest cause) or,
-- with none, its last update. The table holds one row per endpoint, so the backfill is small work.
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD COLUMN "disabled_at" timestamptz NULL;
-- The old enum check does not know "owner"; the next migration adds the new one (NOT VALID, then VALIDATE).
ALTER TABLE "webhook_endpoints" DROP CONSTRAINT "webhook_endpoints_disabled_reason_check";
UPDATE "webhook_endpoints" SET "disabled_reason" = NULL WHERE "enabled";
UPDATE "webhook_endpoints" SET "disabled_at" = COALESCE("last_failure_at", "updated_at"), "disabled_reason" = COALESCE("disabled_reason", 'owner') WHERE NOT "enabled";
-- A state no moment carries would be lost by the drop: refuse instead.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "webhook_endpoints"
    WHERE ("enabled" AND "disabled_at" IS NOT NULL)
       OR (NOT "enabled" AND ("disabled_at" IS NULL OR "disabled_reason" IS NULL))
  ) THEN
    RAISE EXCEPTION 'webhook_endpoints: an enabled state has no moment to carry it; refusing to drop "enabled"';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE "webhook_endpoints" DROP COLUMN "enabled";

-- +goose Down
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD COLUMN "enabled" boolean NOT NULL DEFAULT true;
UPDATE "webhook_endpoints" SET "enabled" = ("disabled_at" IS NULL);
UPDATE "webhook_endpoints" SET "disabled_reason" = NULL WHERE "disabled_reason" = 'owner';
ALTER TABLE "webhook_endpoints" DROP COLUMN "disabled_at";
ALTER TABLE "webhook_endpoints" ADD CONSTRAINT "webhook_endpoints_disabled_reason_check" CHECK ((disabled_reason)::text = ANY ((ARRAY['gone'::character varying, 'persistent_failure'::character varying])::text[])) NOT VALID;
