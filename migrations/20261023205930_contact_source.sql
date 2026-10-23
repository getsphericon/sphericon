-- +goose NO TRANSACTION
-- +goose Up
-- modify "contacts" table
ALTER TABLE "contacts" ADD COLUMN "source" character varying NULL;
ALTER TABLE "contacts" ADD CONSTRAINT "contacts_source_check" CHECK ((source)::text = ANY ((ARRAY['manual'::character varying, 'api'::character varying, 'tracker'::character varying])::text[])) NOT VALID;
ALTER TABLE "contacts" VALIDATE CONSTRAINT "contacts_source_check";

-- +goose Down
-- reverse: modify "contacts" table
ALTER TABLE "contacts" DROP CONSTRAINT "contacts_source_check";
ALTER TABLE "contacts" DROP COLUMN "source";
