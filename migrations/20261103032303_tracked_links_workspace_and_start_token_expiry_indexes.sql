-- +goose NO TRANSACTION
-- +goose Up
-- create index "start_tokens_expires_at" to table: "start_tokens"
CREATE INDEX CONCURRENTLY "start_tokens_expires_at" ON "start_tokens" ("expires_at");
-- create index "trackedlink_workspace_id" to table: "tracked_links"
CREATE INDEX CONCURRENTLY "trackedlink_workspace_id" ON "tracked_links" ("workspace_id");

-- +goose Down
-- reverse: create index "trackedlink_workspace_id" to table: "tracked_links"
DROP INDEX CONCURRENTLY "trackedlink_workspace_id";
-- reverse: create index "start_tokens_expires_at" to table: "start_tokens"
DROP INDEX CONCURRENTLY "start_tokens_expires_at";
