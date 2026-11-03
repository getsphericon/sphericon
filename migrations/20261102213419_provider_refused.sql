-- +goose NO TRANSACTION
-- +goose Up
-- modify "integrations" table: a send that finds its credentials refused holds the Integration
ALTER TABLE "integrations" ADD COLUMN "provider_refused_at" timestamptz NULL;
-- modify "broadcasts" table: a held Broadcast's hold reason gains provider_refused (db:generate does not diff a changed enum CHECK)
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_hold_reason_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_hold_reason_check" CHECK ((hold_reason)::text = ANY ((ARRAY['workspace_suspended'::character varying, 'workspace_pending_deletion'::character varying, 'staff_hold'::character varying, 'no_integration'::character varying, 'unverified_domain'::character varying, 'provider_stopped'::character varying, 'provider_refused'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_hold_reason_check";

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_hold_reason_check";
UPDATE "broadcasts" SET hold_reason = 'provider_stopped' WHERE hold_reason = 'provider_refused';
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_hold_reason_check" CHECK ((hold_reason)::text = ANY ((ARRAY['workspace_suspended'::character varying, 'workspace_pending_deletion'::character varying, 'staff_hold'::character varying, 'no_integration'::character varying, 'unverified_domain'::character varying, 'provider_stopped'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_hold_reason_check";
-- reverse: modify "integrations" table
ALTER TABLE "integrations" DROP COLUMN "provider_refused_at";
