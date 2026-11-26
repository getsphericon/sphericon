-- +goose Up
-- Link identity tokens (ADR 0160): the Workspace's switch and the hostnames of its sites. The checks
-- are added NOT VALID and the next migration (20261126160418) validates them.
ALTER TABLE "workspaces" ADD COLUMN "link_identity_enabled" boolean NOT NULL DEFAULT false, ADD COLUMN "link_identity_hosts" jsonb NOT NULL DEFAULT '[]';
ALTER TABLE "workspaces" ADD CONSTRAINT "workspaces_link_identity_check" CHECK ((NOT link_identity_enabled) OR (jsonb_array_length(link_identity_hosts) > 0)) NOT VALID;
ALTER TABLE "workspaces" ADD CONSTRAINT "workspaces_link_identity_hosts_check" CHECK ((jsonb_typeof(link_identity_hosts) = 'array'::text) AND (NOT (link_identity_hosts @? '$[*]?(@.type() != "string" || !(@ like_regex "^[a-z0-9-]+(\\.[a-z0-9-]+)+$"))'::jsonpath))) NOT VALID;

-- +goose Down
ALTER TABLE "workspaces" DROP CONSTRAINT "workspaces_link_identity_hosts_check", DROP CONSTRAINT "workspaces_link_identity_check";
ALTER TABLE "workspaces" DROP COLUMN "link_identity_hosts", DROP COLUMN "link_identity_enabled";
