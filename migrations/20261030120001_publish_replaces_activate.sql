-- +goose NO TRANSACTION
-- +goose Up
-- Publish replaces Activate for Automations and Rules (ADR 0065, #769). The status is active or
-- inactive: the value inactive is added and every draft row moves to it. draft stays in the CHECK
-- until the breaking ticket of spec #767 removes it (ADR 0033: a migration only adds). ent's diff
-- wrote the two defaults; it does not see a changed enum CHECK, so the constraints and the data
-- moves below are written by hand.
-- modify "automations" table
ALTER TABLE "automations" ALTER COLUMN "status" SET DEFAULT 'inactive';
-- modify "rules" table
ALTER TABLE "rules" ALTER COLUMN "status" SET DEFAULT 'inactive';
ALTER TABLE "automations" DROP CONSTRAINT "automations_status_check";
ALTER TABLE "automations" ADD CONSTRAINT "automations_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying, 'inactive'::character varying])::text[])) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_status_check";
UPDATE "automations" SET "status" = 'inactive' WHERE "status" = 'draft';
ALTER TABLE "rules" DROP CONSTRAINT "rules_status_check";
ALTER TABLE "rules" ADD CONSTRAINT "rules_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying, 'inactive'::character varying])::text[])) NOT VALID;
ALTER TABLE "rules" VALIDATE CONSTRAINT "rules_status_check";
UPDATE "rules" SET "status" = 'inactive' WHERE "status" = 'draft';
-- The send-class scope automations:activate is named automations:publish. A token, Connection,
-- Personal token or pending OAuth code that held it keeps the right under the new name.
UPDATE "api_tokens" SET "scopes" = replace("scopes"::text, '"automations:activate"', '"automations:publish"')::jsonb WHERE "scopes" ? 'automations:activate';
UPDATE "oauth_codes" SET "scopes" = replace("scopes"::text, '"automations:activate"', '"automations:publish"')::jsonb WHERE "scopes" ? 'automations:activate';

-- +goose Down
UPDATE "oauth_codes" SET "scopes" = replace("scopes"::text, '"automations:publish"', '"automations:activate"')::jsonb WHERE "scopes" ? 'automations:publish';
UPDATE "api_tokens" SET "scopes" = replace("scopes"::text, '"automations:publish"', '"automations:activate"')::jsonb WHERE "scopes" ? 'automations:publish';
UPDATE "rules" SET "status" = 'draft' WHERE "status" = 'inactive';
ALTER TABLE "rules" DROP CONSTRAINT "rules_status_check";
ALTER TABLE "rules" ADD CONSTRAINT "rules_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying])::text[])) NOT VALID;
ALTER TABLE "rules" VALIDATE CONSTRAINT "rules_status_check";
UPDATE "automations" SET "status" = 'draft' WHERE "status" = 'inactive';
ALTER TABLE "automations" DROP CONSTRAINT "automations_status_check";
ALTER TABLE "automations" ADD CONSTRAINT "automations_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying])::text[])) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_status_check";
-- reverse: modify "rules" table
ALTER TABLE "rules" ALTER COLUMN "status" SET DEFAULT 'draft';
-- reverse: modify "automations" table
ALTER TABLE "automations" ALTER COLUMN "status" SET DEFAULT 'draft';
