-- +goose NO TRANSACTION
-- +goose Up
-- An accepted Invitation keeps no address (ADR 0050): the Membership and the Audit entry are what
-- remain of the offer, and the address would outlive a User who deletes themself. The accepted rows
-- lose theirs, and the CHECK, added NOT VALID and validated after, keeps it so.
ALTER TABLE "invitations" ALTER COLUMN "email" DROP NOT NULL;
UPDATE "invitations" SET "email" = NULL WHERE "accepted_at" IS NOT NULL;
ALTER TABLE "invitations" ADD CONSTRAINT "invitations_email_check" CHECK ((accepted_at IS NULL) = (email IS NOT NULL)) NOT VALID;
ALTER TABLE "invitations" VALIDATE CONSTRAINT "invitations_email_check";
-- The Audit entries written before the rule named an Invitation, and a Membership raised by one, by
-- its address: they lose it, the one rewrite of an entry's name.
UPDATE "audit_entries" SET "target_name" = NULL WHERE "action" IN ('invitation.create', 'invitation.revoke', 'invitation.accept');
UPDATE "audit_entries" SET "target_name" = NULL WHERE "action" = 'membership.update' AND "target_name" LIKE '%@%';
-- Backs the clearing of an entry's client address and user agent past the client window.
CREATE INDEX CONCURRENTLY "audit_entries_workspace_id_occurred_at_client" ON "audit_entries" ("workspace_id", "occurred_at") WHERE ((ip IS NOT NULL) OR (user_agent IS NOT NULL));

-- +goose Down
DROP INDEX CONCURRENTLY "audit_entries_workspace_id_occurred_at_client";
ALTER TABLE "invitations" DROP CONSTRAINT "invitations_email_check";
UPDATE "invitations" SET "email" = 'accepted-' || "id" || '@invalid' WHERE "email" IS NULL;
ALTER TABLE "invitations" ALTER COLUMN "email" SET NOT NULL;
