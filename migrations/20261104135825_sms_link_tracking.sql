-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "link_tracking" boolean NOT NULL DEFAULT true;
-- modify "tracked_links" table
ALTER TABLE "tracked_links" ALTER COLUMN "outbound_message_id" DROP NOT NULL;

-- +goose Down
-- reverse: modify "tracked_links" table
ALTER TABLE "tracked_links" ALTER COLUMN "outbound_message_id" SET NOT NULL;
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "link_tracking";
