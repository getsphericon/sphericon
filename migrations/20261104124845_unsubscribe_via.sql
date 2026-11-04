-- +goose NO TRANSACTION
-- +goose Up
-- How the person stopped, on an opt-out made in a Channel or by hand (ADR 0077). The CHECK is added
-- NOT VALID and validated after, so the table is not locked while it is checked.
-- modify "unsubscribes" table
ALTER TABLE "unsubscribes" ADD COLUMN "via" character varying NULL;
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_via_check" CHECK ((via)::text = ANY ((ARRAY['keyword'::character varying, 'button'::character varying, 'block'::character varying, 'link'::character varying, 'refused_send'::character varying, 'manual'::character varying])::text[])) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_via_check";
-- Until now an SMS opt-out of everything was only ever recorded by hand (the app or the API).
UPDATE "unsubscribes" SET "via" = 'manual' WHERE "channel" = 'sms' AND "sending_source" = 'everything' AND "via" IS NULL;

-- +goose Down
-- reverse: modify "unsubscribes" table
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_via_check";
ALTER TABLE "unsubscribes" DROP COLUMN "via";
