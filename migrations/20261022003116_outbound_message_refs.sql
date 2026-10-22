-- +goose NO TRANSACTION
-- +goose Up
-- drop index "outbound_messages_broadcast_recipient_id" from table: "outbound_messages" (it was partial; a key needs a whole index)
DROP INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id";
-- create index "outbound_messages_automation_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_automation_id" ON "outbound_messages" ("automation_id");
-- create index "outbound_messages_automation_run_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_automation_run_id" ON "outbound_messages" ("automation_run_id");
-- create index "outbound_messages_broadcast_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_broadcast_id" ON "outbound_messages" ("broadcast_id");
-- create index "outbound_messages_broadcast_recipient_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id" ON "outbound_messages" ("broadcast_recipient_id");
-- create index "outbound_messages_integration_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_integration_id" ON "outbound_messages" ("integration_id");
-- create index "outbound_messages_template_id" to table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_template_id" ON "outbound_messages" ("template_id");
-- modify "outbound_messages" table: the keys are added NOT VALID, so new writes and the deletes of
-- the referenced rows are checked at once. Ids that already dangle (an Automation run or a Template
-- erased before) are cleared by the river job clear_outbound_message_refs, which then validates the
-- keys (ADR 0033: heavy data work is a job; a migration that validated here would fail on them).
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_automation_runs_automation_run" FOREIGN KEY ("automation_run_id") REFERENCES "automation_runs" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_automations_automation" FOREIGN KEY ("automation_id") REFERENCES "automations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient" FOREIGN KEY ("broadcast_recipient_id") REFERENCES "broadcast_recipients" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_broadcasts_broadcast" FOREIGN KEY ("broadcast_id") REFERENCES "broadcasts" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_email_templates_template" FOREIGN KEY ("template_id") REFERENCES "email_templates" ("id") ON UPDATE NO ACTION ON DELETE SET NULL NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_integrations_integration";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_email_templates_template";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_broadcasts_broadcast";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_automations_automation";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_automation_runs_automation_run";
-- reverse: create index "outbound_messages_template_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_template_id";
-- reverse: create index "outbound_messages_integration_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_integration_id";
-- reverse: create index "outbound_messages_broadcast_recipient_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id";
-- reverse: create index "outbound_messages_broadcast_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_broadcast_id";
-- reverse: create index "outbound_messages_automation_run_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_automation_run_id";
-- reverse: create index "outbound_messages_automation_id" to table: "outbound_messages"
DROP INDEX CONCURRENTLY "outbound_messages_automation_id";
-- reverse: drop index "outbound_messages_broadcast_recipient_id" from table: "outbound_messages"
CREATE INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id" ON "outbound_messages" ("broadcast_recipient_id") WHERE (broadcast_recipient_id IS NOT NULL);
