-- +goose NO TRANSACTION
-- +goose Up
-- modify "broadcasts" table: a failed Broadcast keeps when it failed, and its hold reason is one of the declared reasons
ALTER TABLE "broadcasts" ADD COLUMN "failed_at" timestamptz NULL;
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_failed_at_check" CHECK (((status)::text <> 'failed'::text) OR (failed_at IS NOT NULL)) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_failed_at_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_hold_reason_check" CHECK ((hold_reason)::text = ANY ((ARRAY['workspace_suspended'::character varying, 'workspace_pending_deletion'::character varying, 'staff_hold'::character varying, 'no_integration'::character varying, 'unverified_domain'::character varying, 'provider_stopped'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_hold_reason_check";

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "failed_at", DROP CONSTRAINT "broadcasts_hold_reason_check", DROP CONSTRAINT "broadcasts_failed_at_check";
