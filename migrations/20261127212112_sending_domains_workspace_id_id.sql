-- +goose NO TRANSACTION
-- +goose Up
-- The unique key of a Sending domain in its Workspace, which the composite foreign key of the
-- next migration (20261127212737) references: built concurrently, before it.
CREATE UNIQUE INDEX CONCURRENTLY "sending_domains_workspace_id_id" ON "sending_domains" ("workspace_id", "id");

-- +goose Down
DROP INDEX CONCURRENTLY "sending_domains_workspace_id_id";
