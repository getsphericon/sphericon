-- +goose NO TRANSACTION
-- +goose Up
-- The `twilio` provider of the `sms` Channel. ent's diff does not see a changed enum CHECK, so this
-- migration is written by hand.
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_provider_check";
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_provider_check" CHECK ((provider)::text = ANY ((ARRAY['smtp'::character varying, 'ses'::character varying, 'inbox'::character varying, 'telegram'::character varying, 'twilio'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_check";

-- +goose Down
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_provider_check";
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_provider_check" CHECK ((provider)::text = ANY ((ARRAY['smtp'::character varying, 'ses'::character varying, 'inbox'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_check";
