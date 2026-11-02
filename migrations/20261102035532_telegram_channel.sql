-- +goose NO TRANSACTION
-- +goose Up
-- The `telegram` Channel and provider (ADR 0074). ent's diff does not see a changed enum CHECK, so
-- this migration is written by hand.
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_channel_check";
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'sms'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_channel_check";
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_provider_check";
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_provider_check" CHECK ((provider)::text = ANY ((ARRAY['smtp'::character varying, 'ses'::character varying, 'inbox'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_check";
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_channel_check";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_channel_check";
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_channel_check";
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_channel_check";
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_channel_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_channel_check";

-- +goose Down
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_channel_check";
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'sms'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_channel_check";
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_provider_check";
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_provider_check" CHECK ((provider)::text = ANY ((ARRAY['smtp'::character varying, 'ses'::character varying, 'inbox'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_check";
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_channel_check";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_channel_check";
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_channel_check";
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_channel_check";
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_channel_check";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_channel_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_channel_check";
