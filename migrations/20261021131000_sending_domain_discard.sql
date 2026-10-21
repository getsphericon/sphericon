-- +goose NO TRANSACTION
-- +goose Up
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD COLUMN "discarded_at" timestamptz NULL;
-- The partial unique index below no longer supports the workspace foreign key.
CREATE INDEX CONCURRENTLY "sendingdomain_workspace_id" ON "sending_domains" ("workspace_id");
-- Swap the two unique indexes for partial ones without a gap: build the new index under a
-- temporary name, drop the old one, then rename (ADR 0033, ADR 0049).
CREATE UNIQUE INDEX CONCURRENTLY "sendingdomain_workspace_id_domain_kept" ON "sending_domains" ("workspace_id", "domain") WHERE (discarded_at IS NULL);
DROP INDEX CONCURRENTLY "sendingdomain_workspace_id_domain";
ALTER INDEX "sendingdomain_workspace_id_domain_kept" RENAME TO "sendingdomain_workspace_id_domain";
CREATE UNIQUE INDEX CONCURRENTLY "sendingdomain_domain_kept" ON "sending_domains" ("domain") WHERE (tracking_state IN ('dns_verified', 'serving') AND discarded_at IS NULL);
DROP INDEX CONCURRENTLY "sendingdomain_domain";
ALTER INDEX "sendingdomain_domain_kept" RENAME TO "sendingdomain_domain";

-- +goose Down
DROP INDEX CONCURRENTLY "sendingdomain_domain";
CREATE UNIQUE INDEX CONCURRENTLY "sendingdomain_domain" ON "sending_domains" ("domain") WHERE (tracking_state IN ('dns_verified', 'serving'));
DROP INDEX CONCURRENTLY "sendingdomain_workspace_id_domain";
CREATE UNIQUE INDEX CONCURRENTLY "sendingdomain_workspace_id_domain" ON "sending_domains" ("workspace_id", "domain");
DROP INDEX CONCURRENTLY "sendingdomain_workspace_id";
ALTER TABLE "sending_domains" DROP COLUMN "discarded_at";
