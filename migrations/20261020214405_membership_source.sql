-- +goose NO TRANSACTION
-- +goose Up
-- modify "memberships" table
ALTER TABLE "memberships" ADD COLUMN "source" character varying NULL;
ALTER TABLE "memberships" ADD CONSTRAINT "memberships_source_check" CHECK ((source)::text = ANY ((ARRAY['creator'::character varying, 'invitation'::character varying, 'domain'::character varying])::text[])) NOT VALID;
ALTER TABLE "memberships" VALIDATE CONSTRAINT "memberships_source_check";

-- +goose Down
-- reverse: modify "memberships" table
ALTER TABLE "memberships" DROP CONSTRAINT "memberships_source_check";
ALTER TABLE "memberships" DROP COLUMN "source";
