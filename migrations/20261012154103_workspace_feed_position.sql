-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "feed_position" bigint NOT NULL DEFAULT 0;
-- create index "workspace_feed_position_id" to table: "workspaces"
CREATE INDEX "workspace_feed_position_id" ON "workspaces" ("feed_position", "id");

-- +goose Down
-- reverse: create index "workspace_feed_position_id" to table: "workspaces"
DROP INDEX "workspace_feed_position_id";
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "feed_position";
