-- +goose NO TRANSACTION
-- +goose Up
-- An Outbound message on `sms` (ADR 0079). ent's diff does not see a changed enum CHECK, so this
-- migration is written by hand.
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_channel_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying, 'sms'::character varying])::text[])) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_channel_check";

-- +goose Down
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_channel_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_channel_check";
