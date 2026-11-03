-- +goose NO TRANSACTION
-- +goose Up
-- modify "broadcasts" table: a Broadcast can be on the telegram Channel, whose content is a Message
-- content: a text, an image, link buttons, the link preview and an unsubscribe button. ent's diff does
-- not see a changed enum CHECK or content CHECK, so those two are written by hand.
ALTER TABLE "broadcasts" ADD COLUMN "text" character varying NULL, ADD COLUMN "image_url" character varying NULL, ADD COLUMN "buttons" jsonb NULL, ADD COLUMN "link_preview" boolean NULL, ADD COLUMN "unsubscribe_button" character varying NULL;
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'telegram'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_content_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_content_check" CHECK ((((channel)::text = 'email'::text) AND (subject IS NOT NULL) AND (body IS NOT NULL) AND (text IS NULL) AND (buttons IS NULL) AND (link_preview IS NULL) AND (image_url IS NULL) AND (unsubscribe_button IS NULL)) OR (((channel)::text = 'telegram'::text) AND (text IS NOT NULL) AND (buttons IS NOT NULL) AND (link_preview IS NOT NULL) AND (subject IS NULL) AND (body IS NULL) AND (from_name IS NULL) AND (from_email IS NULL) AND (doc IS NULL) AND (editor_version IS NULL))) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_content_check";

-- +goose Down
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_content_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" DROP COLUMN "unsubscribe_button", DROP COLUMN "link_preview", DROP COLUMN "buttons", DROP COLUMN "image_url", DROP COLUMN "text";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_content_check" CHECK (((channel)::text = 'email'::text) AND (subject IS NOT NULL) AND (body IS NOT NULL)) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_content_check";
