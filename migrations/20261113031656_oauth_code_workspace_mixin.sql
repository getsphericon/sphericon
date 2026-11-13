-- +goose Up
-- An authorization code is Workspace-owned (WorkspaceMixin): its key to the Workspace is the
-- mixin's, which the erase of a Workspace removes the codes before, as every Workspace-owned row.
-- Added NOT VALID under the runner's bounded lock wait, and validated by the next migration.
ALTER TABLE "oauth_codes" DROP CONSTRAINT "oauth_codes_workspaces_workspace", ADD CONSTRAINT "oauth_codes_workspaces_oauth_codes" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION NOT VALID;

-- +goose Down
ALTER TABLE "oauth_codes" DROP CONSTRAINT "oauth_codes_workspaces_oauth_codes", ADD CONSTRAINT "oauth_codes_workspaces_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "oauth_codes" VALIDATE CONSTRAINT "oauth_codes_workspaces_workspace";
