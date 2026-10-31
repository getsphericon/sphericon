-- +goose Up
-- modify "automations" table
ALTER TABLE "automations" ADD COLUMN "activated_at" timestamptz NULL;
-- modify "rule_firings" table
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "rule_firings" ALTER COLUMN "version_id" SET NOT NULL;

-- +goose Down
-- reverse: modify "rule_firings" table
ALTER TABLE "rule_firings" ALTER COLUMN "version_id" DROP NOT NULL;
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP COLUMN "activated_at";
