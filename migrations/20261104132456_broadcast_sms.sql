-- +goose NO TRANSACTION
-- +goose Up
-- A Broadcast can be on the sms Channel, whose content is the text alone, in the column a Telegram
-- Broadcast's text is in. ent's diff does not see a changed enum CHECK or content CHECK, so this
-- migration is written by hand.
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying, 'sms'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_content_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_content_check" CHECK ((((channel)::text = 'email'::text) AND (subject IS NOT NULL) AND (body IS NOT NULL) AND (text IS NULL) AND (buttons IS NULL) AND (image_url IS NULL) AND (unsubscribe_button IS NULL)) OR (((channel)::text = 'telegram'::text) AND (text IS NOT NULL) AND (buttons IS NOT NULL) AND (subject IS NULL) AND (body IS NULL) AND (from_name IS NULL) AND (from_email IS NULL) AND (doc IS NULL) AND (editor_version IS NULL)) OR (((channel)::text = 'sms'::text) AND (text IS NOT NULL) AND (subject IS NULL) AND (body IS NULL) AND (from_name IS NULL) AND (from_email IS NULL) AND (doc IS NULL) AND (editor_version IS NULL) AND (buttons IS NULL) AND (image_url IS NULL) AND (unsubscribe_button IS NULL))) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_content_check";

-- +goose Down
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_content_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_content_check" CHECK ((((channel)::text = 'email'::text) AND (subject IS NOT NULL) AND (body IS NOT NULL) AND (text IS NULL) AND (buttons IS NULL) AND (image_url IS NULL) AND (unsubscribe_button IS NULL)) OR (((channel)::text = 'telegram'::text) AND (text IS NOT NULL) AND (buttons IS NOT NULL) AND (subject IS NULL) AND (body IS NULL) AND (from_name IS NULL) AND (from_email IS NULL) AND (doc IS NULL) AND (editor_version IS NULL))) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_content_check";
