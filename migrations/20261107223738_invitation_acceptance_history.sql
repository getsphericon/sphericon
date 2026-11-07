-- +goose NO TRANSACTION
-- +goose Up
-- Accepted Invitations keep their acceptance. Build the pending-only key before removing the
-- old key so there is no window in which two unaccepted Invitations can share an address.
CREATE UNIQUE INDEX CONCURRENTLY "invitations_pending_workspace_email" ON "invitations" ("workspace_id", "email") WHERE (accepted_at IS NULL);
DROP INDEX CONCURRENTLY "invitation_workspace_id_email";

-- +goose Down
-- Reinstating the stricter key fails before the pending key is removed if an address now has
-- accepted history as well as another Invitation. No Invitation is removed by this migration.
CREATE UNIQUE INDEX CONCURRENTLY "invitation_workspace_id_email" ON "invitations" ("workspace_id", "email");
DROP INDEX CONCURRENTLY "invitations_pending_workspace_email";
