-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID.
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_link_identity_check";
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_link_identity_hosts_check";

-- +goose Down
-- Nothing: the previous migration's Down drops the checks.
