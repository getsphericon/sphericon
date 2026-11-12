-- +goose Up
-- modify "automations" table
ALTER TABLE "automations" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "brand_kits" table
ALTER TABLE "brand_kits" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "contacts" table
ALTER TABLE "contacts" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "custom_fields" table
ALTER TABLE "custom_fields" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "email_templates" table
ALTER TABLE "email_templates" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "integrations" table
ALTER TABLE "integrations" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "rules" table
ALTER TABLE "rules" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "segments" table
ALTER TABLE "segments" ALTER COLUMN "revision" DROP DEFAULT;
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ALTER COLUMN "revision" DROP DEFAULT;

-- +goose Down
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "segments" table
ALTER TABLE "segments" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "rules" table
ALTER TABLE "rules" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "integrations" table
ALTER TABLE "integrations" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "email_templates" table
ALTER TABLE "email_templates" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "custom_fields" table
ALTER TABLE "custom_fields" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "contacts" table
ALTER TABLE "contacts" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "brand_kits" table
ALTER TABLE "brand_kits" ALTER COLUMN "revision" SET DEFAULT 1;
-- reverse: modify "automations" table
ALTER TABLE "automations" ALTER COLUMN "revision" SET DEFAULT 1;
