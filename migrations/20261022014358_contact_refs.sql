-- +goose NO TRANSACTION
-- +goose Up
-- Whole indexes on contact_id are built CONCURRENTLY (a key's supporting index cannot be partial), the
-- old partial ones go, then the keys are added NOT VALID: new writes and Contact deletes are checked at
-- once. Ids that already dangle are cleared by the river job clear_contact_refs, which then validates
-- the keys (ADR 0033: heavy data work is a job; validating here would fail on them).
-- create index "automation_runs_contact_id" to table: "automation_runs"
CREATE INDEX CONCURRENTLY "automation_runs_contact_id" ON "automation_runs" ("contact_id");
-- create index "broadcast_recipients_contact_id" to table: "broadcast_recipients"
CREATE INDEX CONCURRENTLY "broadcast_recipients_contact_id" ON "broadcast_recipients" ("contact_id");
-- create index "confirmations_contact_id" to table: "confirmations"
CREATE INDEX CONCURRENTLY "confirmations_contact_id" ON "confirmations" ("contact_id");
-- create index "outbound_messages_contact_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_contact_id" ON "outbound_messages" ("contact_id");
-- create index "suppressions_contact_id" to table: "suppressions"
CREATE INDEX CONCURRENTLY "suppressions_contact_id" ON "suppressions" ("contact_id");
-- create index "unsubscribes_contact_id" to table: "unsubscribes"
CREATE INDEX CONCURRENTLY "unsubscribes_contact_id" ON "unsubscribes" ("contact_id");
-- drop the partial indexes the whole ones replace
DROP INDEX CONCURRENTLY "broadcast_recipients_workspace_id_contact_id";
DROP INDEX CONCURRENTLY "confirmations_workspace_id_contact_id";
DROP INDEX CONCURRENTLY "outbound_messages_workspace_id_contact_id";
DROP INDEX CONCURRENTLY "unsubscribes_workspace_id_contact_id";
-- modify the tables: the keys, NOT VALID
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "broadcast_recipients" ADD CONSTRAINT "broadcast_recipients_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;

-- +goose Down
-- reverse: modify the tables
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_contacts_contact";
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_contacts_contact";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_contacts_contact";
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_contacts_contact";
ALTER TABLE "broadcast_recipients" DROP CONSTRAINT "broadcast_recipients_contacts_contact";
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_contacts_contact";
-- reverse: drop the partial indexes
CREATE INDEX CONCURRENTLY "unsubscribes_workspace_id_contact_id" ON "unsubscribes" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_contact_id" ON "outbound_messages" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
CREATE INDEX CONCURRENTLY "confirmations_workspace_id_contact_id" ON "confirmations" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
CREATE INDEX CONCURRENTLY "broadcast_recipients_workspace_id_contact_id" ON "broadcast_recipients" ("workspace_id", "contact_id") WHERE (contact_id IS NOT NULL);
-- reverse: create the whole indexes
DROP INDEX CONCURRENTLY "unsubscribes_contact_id";
DROP INDEX CONCURRENTLY "suppressions_contact_id";
DROP INDEX CONCURRENTLY "outbound_messages_contact_id";
DROP INDEX CONCURRENTLY "confirmations_contact_id";
DROP INDEX CONCURRENTLY "broadcast_recipients_contact_id";
DROP INDEX CONCURRENTLY "automation_runs_contact_id";
