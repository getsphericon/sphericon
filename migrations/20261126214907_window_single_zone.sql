-- +goose Up
-- modify "automation_figures" table
ALTER TABLE "automation_figures" ADD COLUMN "single_zone" bigint NOT NULL DEFAULT 0;
-- modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" ADD COLUMN "single_zone" bigint NOT NULL DEFAULT 0;
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "window_single_zone" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "window_single_zone";
-- reverse: modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" DROP COLUMN "single_zone";
-- reverse: modify "automation_figures" table
ALTER TABLE "automation_figures" DROP COLUMN "single_zone";
