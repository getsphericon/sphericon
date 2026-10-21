-- +goose NO TRANSACTION
-- +goose Up
-- create index "automationrun_workspace_id_contact_id" to table: "automation_runs"
CREATE INDEX CONCURRENTLY "automationrun_workspace_id_contact_id" ON "automation_runs" ("workspace_id", "contact_id");
-- drop index "automationrun_workspace_id" from table: "automation_runs"
DROP INDEX CONCURRENTLY "automationrun_workspace_id";
-- create index "broadcast_recipients_workspace_id_contact_id" to table: "broadcast_recipients"
CREATE INDEX CONCURRENTLY "broadcast_recipients_workspace_id_contact_id" ON "broadcast_recipients" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
-- create index "confirmations_workspace_id_contact_id" to table: "confirmations"
CREATE INDEX CONCURRENTLY "confirmations_workspace_id_contact_id" ON "confirmations" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
-- create index "outbound_messages_workspace_id_contact_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_contact_id" ON "outbound_messages" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
-- create index "suppressions_workspace_id_contact_id" to table: "suppressions"
CREATE INDEX CONCURRENTLY "suppressions_workspace_id_contact_id" ON "suppressions" ("workspace_id", "contact_id");
-- create index "unsubscribes_workspace_id_contact_id" to table: "unsubscribes"
CREATE INDEX CONCURRENTLY "unsubscribes_workspace_id_contact_id" ON "unsubscribes" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
-- create index "oauthcode_workspace_id" to table: "oauth_codes"
CREATE INDEX CONCURRENTLY "oauthcode_workspace_id" ON "oauth_codes" ("workspace_id");
-- modify "oauth_codes" table: a code outlives an erased Workspace only as a dangling row
DELETE FROM "oauth_codes" WHERE "workspace_id" NOT IN (SELECT "id" FROM "workspaces");
ALTER TABLE "oauth_codes" ADD CONSTRAINT "oauth_codes_workspaces_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "oauth_codes" VALIDATE CONSTRAINT "oauth_codes_workspaces_workspace";

-- +goose Down
-- reverse: modify "oauth_codes" table
ALTER TABLE "oauth_codes" DROP CONSTRAINT "oauth_codes_workspaces_workspace";
-- reverse: create index "oauthcode_workspace_id" to table: "oauth_codes"
DROP INDEX CONCURRENTLY "oauthcode_workspace_id";
-- reverse: create index "unsubscribes_workspace_id_contact_id" to table: "unsubscribes"
DROP INDEX CONCURRENTLY "unsubscribes_workspace_id_contact_id";
-- reverse: create index "suppressions_workspace_id_contact_id" to table: "suppressions"
DROP INDEX CONCURRENTLY "suppressions_workspace_id_contact_id";
-- reverse: create index "outbound_messages_workspace_id_contact_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_workspace_id_contact_id";
-- reverse: create index "confirmations_workspace_id_contact_id" to table: "confirmations"
DROP INDEX CONCURRENTLY "confirmations_workspace_id_contact_id";
-- reverse: create index "broadcast_recipients_workspace_id_contact_id" to table: "broadcast_recipients"
DROP INDEX CONCURRENTLY "broadcast_recipients_workspace_id_contact_id";
-- reverse: create index "automationrun_workspace_id_contact_id" to table: "automation_runs"
DROP INDEX CONCURRENTLY "automationrun_workspace_id_contact_id";
-- reverse: drop index "automationrun_workspace_id" from table: "automation_runs"
CREATE INDEX CONCURRENTLY "automationrun_workspace_id" ON "automation_runs" ("workspace_id");
