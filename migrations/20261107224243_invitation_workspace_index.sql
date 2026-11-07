-- +goose NO TRANSACTION
-- +goose Up
-- create index "invitation_workspace_id" to table: "invitations"
CREATE INDEX CONCURRENTLY "invitation_workspace_id" ON "invitations" ("workspace_id");

-- +goose Down
-- reverse: create index "invitation_workspace_id" to table: "invitations"
DROP INDEX CONCURRENTLY "invitation_workspace_id";
