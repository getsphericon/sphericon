-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check of the Engagement tracking default the previous migration added NOT VALID.
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_engagement_tracking_default_check";

-- +goose Down
