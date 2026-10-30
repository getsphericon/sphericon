-- +goose NO TRANSACTION
-- +goose Up
-- modify "users" table
ALTER TABLE "users" ADD COLUMN "feed_position" bigint NOT NULL DEFAULT 0;
-- create index "user_feed_position_id" to table: "users"
CREATE INDEX CONCURRENTLY "user_feed_position_id" ON "users" ("feed_position", "id");

-- +goose Down
-- reverse: create index "user_feed_position_id" to table: "users"
DROP INDEX CONCURRENTLY "user_feed_position_id";
-- reverse: modify "users" table
ALTER TABLE "users" DROP COLUMN "feed_position";
