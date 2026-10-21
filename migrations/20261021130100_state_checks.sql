-- +goose NO TRANSACTION
-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_scheduled_at_check" CHECK (((status)::text <> 'scheduled'::text) OR (scheduled_at IS NOT NULL)) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_scheduled_at_check";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_sent_at_check" CHECK (((status)::text <> 'sent'::text) OR (sent_at IS NOT NULL)) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_sent_at_check";
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD CONSTRAINT "sending_domains_verified_check" CHECK ((NOT verified) OR (verified_at IS NOT NULL)) NOT VALID;
ALTER TABLE "sending_domains" VALIDATE CONSTRAINT "sending_domains_verified_check";
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD CONSTRAINT "workspaces_suspension_check" CHECK (((suspended_at IS NULL) = (suspended_by_kind IS NULL)) AND ((suspended_at IS NULL) = (suspension_reason IS NULL)) AND (COALESCE(((suspended_by_kind)::text = 'staff'::text), false) = (suspended_by_id IS NOT NULL))) NOT VALID;
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_suspension_check";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP CONSTRAINT "workspaces_suspension_check";
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP CONSTRAINT "sending_domains_verified_check";
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_sent_at_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_scheduled_at_check";
