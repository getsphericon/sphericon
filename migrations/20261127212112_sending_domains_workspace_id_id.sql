-- +goose NO TRANSACTION
-- +goose Up
-- The unique key of a Sending domain in its Workspace, which the composite foreign key of the
-- next migration (20261127212737) references: built concurrently, before it. It leads with
-- workspace_id, so it also serves the Workspace's foreign key, and the index on workspace_id alone
-- goes, last.
CREATE UNIQUE INDEX CONCURRENTLY "sending_domains_workspace_id_id" ON "sending_domains" ("workspace_id", "id");
DROP INDEX CONCURRENTLY "sendingdomain_workspace_id";

-- +goose Down
CREATE INDEX CONCURRENTLY "sendingdomain_workspace_id" ON "sending_domains" ("workspace_id");
DROP INDEX CONCURRENTLY "sending_domains_workspace_id_id";
