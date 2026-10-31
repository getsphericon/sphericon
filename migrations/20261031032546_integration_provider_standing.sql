-- +goose NO TRANSACTION
-- +goose Up
-- modify "integrations" table: the provider account's standing, read with its quota (ADR 0067).
-- The CHECK ent's diff wrote is added NOT VALID and validated after, by hand.
ALTER TABLE "integrations" ADD COLUMN "provider_enforcement" character varying NULL, ADD COLUMN "provider_sandbox" boolean NOT NULL DEFAULT false;
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_provider_enforcement_check" CHECK ((provider_enforcement)::text = ANY ((ARRAY['probation'::character varying, 'shutdown'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_enforcement_check";

-- +goose Down
-- reverse: modify "integrations" table
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_provider_enforcement_check", DROP COLUMN "provider_sandbox", DROP COLUMN "provider_enforcement";
