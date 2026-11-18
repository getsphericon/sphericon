-- +goose Up
-- modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" ADD COLUMN "cancelled" bigint NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" DROP COLUMN "cancelled";
