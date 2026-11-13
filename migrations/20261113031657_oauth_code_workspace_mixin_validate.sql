-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE "oauth_codes" VALIDATE CONSTRAINT "oauth_codes_workspaces_oauth_codes";

-- +goose Down
ALTER TABLE "oauth_codes" VALIDATE CONSTRAINT "oauth_codes_workspaces_workspace";
