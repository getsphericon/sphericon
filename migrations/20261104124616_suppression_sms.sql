-- +goose NO TRANSACTION
-- +goose Up
-- A Suppression on `sms`: a number Twilio reports reaches no handset is suppressed like a hard
-- bounce (#1032). ent's diff does not see a changed enum CHECK, so this migration is written by hand.
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_channel_check";
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying, 'sms'::character varying])::text[])) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_channel_check";

-- +goose Down
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_channel_check";
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_channel_check";
