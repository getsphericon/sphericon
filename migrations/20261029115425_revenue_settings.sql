-- +goose Up
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD COLUMN "reporting_currency" character varying NULL, ADD COLUMN "attribution_window_days" bigint NOT NULL DEFAULT 5;

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP COLUMN "attribution_window_days", DROP COLUMN "reporting_currency";
