-- +goose Up
-- Breaking (ADR 0033, ADR 0067): the Send rate limit is the Provider limit, so the table of its
-- token buckets is provider_limiters. ent's diff would create the new table and leave the old one;
-- this renames it in place instead, with its keys, indexes and sequence, so the buckets carry over
-- and nothing is stranded.
ALTER TABLE "send_limiters" RENAME TO "provider_limiters";
ALTER TABLE "provider_limiters" RENAME CONSTRAINT "send_limiters_pkey" TO "provider_limiters_pkey";
ALTER TABLE "provider_limiters" RENAME CONSTRAINT "send_limiters_integrations_send_limiter" TO "provider_limiters_integrations_provider_limiter";
ALTER TABLE "provider_limiters" RENAME CONSTRAINT "send_limiters_workspaces_send_limiters" TO "provider_limiters_workspaces_provider_limiters";
ALTER INDEX "send_limiters_integration_id_key" RENAME TO "provider_limiters_integration_id_key";
ALTER INDEX "sendlimiter_workspace_id" RENAME TO "providerlimiter_workspace_id";
ALTER SEQUENCE "send_limiters_id_seq" RENAME TO "provider_limiters_id_seq";

-- +goose Down
ALTER SEQUENCE "provider_limiters_id_seq" RENAME TO "send_limiters_id_seq";
ALTER INDEX "providerlimiter_workspace_id" RENAME TO "sendlimiter_workspace_id";
ALTER INDEX "provider_limiters_integration_id_key" RENAME TO "send_limiters_integration_id_key";
ALTER TABLE "provider_limiters" RENAME CONSTRAINT "provider_limiters_workspaces_provider_limiters" TO "send_limiters_workspaces_send_limiters";
ALTER TABLE "provider_limiters" RENAME CONSTRAINT "provider_limiters_integrations_provider_limiter" TO "send_limiters_integrations_send_limiter";
ALTER TABLE "provider_limiters" RENAME CONSTRAINT "provider_limiters_pkey" TO "send_limiters_pkey";
ALTER TABLE "provider_limiters" RENAME TO "send_limiters";
