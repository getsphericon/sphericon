-- +goose Up
-- A Broadcast's frozen figures keep its untracked messages (ADR 0120): the sent messages that were
-- not tracked at freeze. A row frozen before reads 0, as every message then went out tracked.
ALTER TABLE "broadcast_figures" ADD COLUMN "untracked" bigint NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" DROP COLUMN "untracked";
