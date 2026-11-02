-- +goose NO TRANSACTION
-- +goose Up
-- modify "broadcasts" table: a Broadcast has a Channel, fixed at creation (the rows before it are email), and holds exactly that Channel's content
ALTER TABLE "broadcasts" ADD COLUMN "channel" character varying NOT NULL DEFAULT 'email';
ALTER TABLE "broadcasts" ALTER COLUMN "subject" DROP NOT NULL, ALTER COLUMN "subject" DROP DEFAULT, ALTER COLUMN "body" DROP NOT NULL, ALTER COLUMN "body" DROP DEFAULT;
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_channel_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_content_check" CHECK (((channel)::text = 'email'::text) AND (subject IS NOT NULL) AND (body IS NOT NULL)) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_content_check";

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "channel", ALTER COLUMN "body" SET NOT NULL, ALTER COLUMN "body" SET DEFAULT '', ALTER COLUMN "subject" SET NOT NULL, ALTER COLUMN "subject" SET DEFAULT '', DROP CONSTRAINT "broadcasts_content_check", DROP CONSTRAINT "broadcasts_channel_check";
