-- +goose Up
-- An Integration's quota failure becomes provider_quota_failed_at, the moment the lookup first failed
-- (nil = no failure), instead of the provider_quota_unavailable flag (ADR 0033, review rule 7). The
-- flag column is removed, so a Major is required. The flag never kept when the failure began, so the
-- best moment left is the last check (the lookup that set the flag) or, with none, the last update. The
-- table holds a handful of rows per Workspace, so the backfill is small work.
-- modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "provider_quota_failed_at" timestamptz NULL;
UPDATE "integrations" SET "provider_quota_failed_at" = COALESCE("provider_quota_checked_at", "updated_at") WHERE "provider_quota_unavailable";
-- A failure no moment carries would be lost by the drop: refuse instead.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM "integrations"
    WHERE "provider_quota_unavailable" <> ("provider_quota_failed_at" IS NOT NULL)
  ) THEN
    RAISE EXCEPTION 'integrations: a quota failure has no moment to carry it; refusing to drop "provider_quota_unavailable"';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE "integrations" DROP COLUMN "provider_quota_unavailable";

-- +goose Down
-- reverse: modify "integrations" table
ALTER TABLE "integrations" ADD COLUMN "provider_quota_unavailable" boolean NOT NULL DEFAULT false;
UPDATE "integrations" SET "provider_quota_unavailable" = ("provider_quota_failed_at" IS NOT NULL);
ALTER TABLE "integrations" DROP COLUMN "provider_quota_failed_at";
