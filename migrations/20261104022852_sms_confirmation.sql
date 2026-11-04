-- +goose NO TRANSACTION
-- +goose Up
-- A Confirmation on `sms` with its evidence, the `keyword` provenance, and an opt-out on `sms`
-- (ADR 0077). ent's diff does not see a changed enum CHECK, so those statements are written by hand.
-- modify "confirmations" table
ALTER TABLE "confirmations" ADD COLUMN "wording" character varying NULL, ADD COLUMN "source" character varying NULL, ADD COLUMN "brand" character varying NULL;
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_channel_check";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying, 'sms'::character varying])::text[])) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_channel_check";
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_provenance_check";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_provenance_check" CHECK ((provenance)::text = ANY ((ARRAY['double_opt_in'::character varying, 'grandfathered'::character varying, 'imported'::character varying, 'keyword'::character varying])::text[])) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_provenance_check";
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying, 'sms'::character varying])::text[])) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_channel_check";

-- +goose Down
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_provenance_check";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_provenance_check" CHECK ((provenance)::text = ANY ((ARRAY['double_opt_in'::character varying, 'grandfathered'::character varying, 'imported'::character varying])::text[])) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_provenance_check";
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_channel_check";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_channel_check";
-- reverse: modify "confirmations" table
ALTER TABLE "confirmations" DROP COLUMN "brand", DROP COLUMN "source", DROP COLUMN "wording";
