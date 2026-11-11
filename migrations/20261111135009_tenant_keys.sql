-- +goose NO TRANSACTION
-- +goose Up
-- Every reference between two Workspace-owned tables carries the Workspace (ADR 0017): the foreign
-- key is (workspace_id, x_id) -> parent (workspace_id, id), so the database refuses a row of one
-- Workspace pointing at another's; a SET NULL key clears only x_id. Each parent gets the unique key
-- (workspace_id, id), and each child index that served a reference now leads with workspace_id.
-- Indexes are built first and dropped last, concurrently; each key is added NOT VALID and validated
-- after, so no table is locked while its rows are checked.
CREATE INDEX CONCURRENTLY "apitoken_workspace_id_membership_id" ON "api_tokens" ("workspace_id", "membership_id");
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_workspace_id_automation_id_contact_id" ON "automation_runs" ("workspace_id", "automation_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "automation_runs_workspace_id_id" ON "automation_runs" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "automationrun_workspace_id_version_id" ON "automation_runs" ("workspace_id", "version_id");
CREATE UNIQUE INDEX CONCURRENTLY "automation_versions_workspace_id_id" ON "automation_versions" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "automation_workspace_id_draft_version_id" ON "automations" ("workspace_id", "draft_version_id");
CREATE INDEX CONCURRENTLY "automation_workspace_id_live_version_id" ON "automations" ("workspace_id", "live_version_id");
CREATE UNIQUE INDEX CONCURRENTLY "automations_workspace_id_id" ON "automations" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "bindings_workspace_id_contact_id" ON "bindings" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "bindings_workspace_id_integration_destination" ON "bindings" ("workspace_id", "integration_id", "destination");
CREATE INDEX CONCURRENTLY "bindings_workspace_id_visitor_id" ON "bindings" ("workspace_id", "visitor_id");
CREATE UNIQUE INDEX CONCURRENTLY "broadcast_recipients_workspace_id_broadcast_id_contact_id" ON "broadcast_recipients" ("workspace_id", "broadcast_id", "contact_id");
CREATE INDEX CONCURRENTLY "broadcast_recipients_workspace_id_contact_id" ON "broadcast_recipients" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "broadcast_recipients_workspace_id_id" ON "broadcast_recipients" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "broadcast_workspace_id_integration_id" ON "broadcasts" ("workspace_id", "integration_id");
CREATE INDEX CONCURRENTLY "broadcast_workspace_id_segment_id" ON "broadcasts" ("workspace_id", "segment_id");
CREATE UNIQUE INDEX CONCURRENTLY "broadcasts_workspace_id_id" ON "broadcasts" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "captured_messages_workspace_id_integration_id" ON "captured_messages" ("workspace_id", "integration_id");
CREATE INDEX CONCURRENTLY "captured_messages_workspace_id_outbound_message_id" ON "captured_messages" ("workspace_id", "outbound_message_id");
CREATE INDEX CONCURRENTLY "confirmations_workspace_id_contact_id" ON "confirmations" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "contacts_workspace_id_id" ON "contacts" ("workspace_id", "id");
CREATE UNIQUE INDEX CONCURRENTLY "email_templates_workspace_id_id" ON "email_templates" ("workspace_id", "id");
CREATE UNIQUE INDEX CONCURRENTLY "integrations_workspace_id_id" ON "integrations" ("workspace_id", "id");
CREATE UNIQUE INDEX CONCURRENTLY "memberships_workspace_id_id" ON "memberships" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "oauthcode_workspace_id_membership_id" ON "oauth_codes" ("workspace_id", "membership_id");
CREATE INDEX CONCURRENTLY "ordermovement_workspace_id_order_id" ON "order_movements" ("workspace_id", "order_id");
CREATE INDEX CONCURRENTLY "order_workspace_id_automation_id" ON "orders" ("workspace_id", "automation_id");
CREATE INDEX CONCURRENTLY "order_workspace_id_broadcast_id" ON "orders" ("workspace_id", "broadcast_id");
CREATE INDEX CONCURRENTLY "order_workspace_id_contact_id" ON "orders" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "orders_workspace_id_id" ON "orders" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_automation_id" ON "outbound_messages" ("workspace_id", "automation_id");
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_automation_run_id" ON "outbound_messages" ("workspace_id", "automation_run_id");
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_broadcast_recipient_id" ON "outbound_messages" ("workspace_id", "broadcast_recipient_id");
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_contact_id" ON "outbound_messages" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "outbound_messages_workspace_id_id" ON "outbound_messages" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "outbound_messages_workspace_id_template_id" ON "outbound_messages" ("workspace_id", "template_id");
CREATE INDEX CONCURRENTLY "providerlimiter_workspace_id_integration_id" ON "provider_limiters" ("workspace_id", "integration_id");
CREATE INDEX CONCURRENTLY "rule_firings_workspace_id_contact_id" ON "rule_firings" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "rulefiring_workspace_id_rule_id_source_id" ON "rule_firings" ("workspace_id", "rule_id", "source_id");
CREATE INDEX CONCURRENTLY "rulefiring_workspace_id_version_id" ON "rule_firings" ("workspace_id", "version_id");
CREATE UNIQUE INDEX CONCURRENTLY "ruleversiontrigger_workspace_id_version_id_action" ON "rule_version_triggers" ("workspace_id", "version_id", "action");
CREATE UNIQUE INDEX CONCURRENTLY "rule_versions_workspace_id_id" ON "rule_versions" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "rule_workspace_id_draft_version_id" ON "rules" ("workspace_id", "draft_version_id");
CREATE INDEX CONCURRENTLY "rule_workspace_id_live_version_id" ON "rules" ("workspace_id", "live_version_id");
CREATE UNIQUE INDEX CONCURRENTLY "rules_workspace_id_id" ON "rules" ("workspace_id", "id");
CREATE UNIQUE INDEX CONCURRENTLY "segments_workspace_id_id" ON "segments" ("workspace_id", "id");
CREATE INDEX CONCURRENTLY "start_tokens_workspace_id_contact_id" ON "start_tokens" ("workspace_id", "contact_id");
CREATE INDEX CONCURRENTLY "start_tokens_workspace_id_integration_id" ON "start_tokens" ("workspace_id", "integration_id");
CREATE INDEX CONCURRENTLY "start_tokens_workspace_id_visitor_id" ON "start_tokens" ("workspace_id", "visitor_id");
CREATE INDEX CONCURRENTLY "tracked_links_workspace_id_outbound_message_id" ON "tracked_links" ("workspace_id", "outbound_message_id");
CREATE INDEX CONCURRENTLY "unsubscribes_workspace_id_contact_id" ON "unsubscribes" ("workspace_id", "contact_id");
CREATE INDEX CONCURRENTLY "visitor_workspace_id_contact_id" ON "visitors" ("workspace_id", "contact_id");
CREATE UNIQUE INDEX CONCURRENTLY "visitors_workspace_id_id" ON "visitors" ("workspace_id", "id");
-- modify "api_tokens" table
ALTER TABLE "api_tokens" DROP CONSTRAINT "api_tokens_memberships_connections";
ALTER TABLE "api_tokens" ADD CONSTRAINT "api_tokens_memberships_connections" FOREIGN KEY ("workspace_id", "membership_id") REFERENCES "memberships" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "api_tokens" VALIDATE CONSTRAINT "api_tokens_memberships_connections";
-- modify "automation_runs" table
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_automation_versions_runs";
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_automations_runs";
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_contacts_contact";
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_automation_versions_runs" FOREIGN KEY ("workspace_id", "version_id") REFERENCES "automation_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE NO ACTION NOT VALID;
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_automation_versions_runs";
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_automations_runs" FOREIGN KEY ("workspace_id", "automation_id") REFERENCES "automations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_automations_runs";
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_contacts_contact";
-- modify "automation_versions" table
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_automations_versions";
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_automations_versions" FOREIGN KEY ("workspace_id", "automation_id") REFERENCES "automations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_automations_versions";
-- modify "automations" table
ALTER TABLE "automations" DROP CONSTRAINT "automations_automation_versions_draft_version";
ALTER TABLE "automations" DROP CONSTRAINT "automations_automation_versions_live_version";
ALTER TABLE "automations" ADD CONSTRAINT "automations_automation_versions_draft_version" FOREIGN KEY ("workspace_id", "draft_version_id") REFERENCES "automation_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (draft_version_id) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_automation_versions_draft_version";
ALTER TABLE "automations" ADD CONSTRAINT "automations_automation_versions_live_version" FOREIGN KEY ("workspace_id", "live_version_id") REFERENCES "automation_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (live_version_id) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_automation_versions_live_version";
-- modify "bindings" table
ALTER TABLE "bindings" DROP CONSTRAINT "bindings_contacts_bindings";
ALTER TABLE "bindings" DROP CONSTRAINT "bindings_integrations_bindings";
ALTER TABLE "bindings" DROP CONSTRAINT "bindings_visitors_bindings";
ALTER TABLE "bindings" ADD CONSTRAINT "bindings_contacts_bindings" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "bindings" VALIDATE CONSTRAINT "bindings_contacts_bindings";
ALTER TABLE "bindings" ADD CONSTRAINT "bindings_integrations_bindings" FOREIGN KEY ("workspace_id", "integration_id") REFERENCES "integrations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "bindings" VALIDATE CONSTRAINT "bindings_integrations_bindings";
ALTER TABLE "bindings" ADD CONSTRAINT "bindings_visitors_bindings" FOREIGN KEY ("workspace_id", "visitor_id") REFERENCES "visitors" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "bindings" VALIDATE CONSTRAINT "bindings_visitors_bindings";
-- modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" DROP CONSTRAINT "broadcast_recipients_broadcasts_recipients";
ALTER TABLE "broadcast_recipients" DROP CONSTRAINT "broadcast_recipients_contacts_contact";
ALTER TABLE "broadcast_recipients" ADD CONSTRAINT "broadcast_recipients_broadcasts_recipients" FOREIGN KEY ("workspace_id", "broadcast_id") REFERENCES "broadcasts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE NO ACTION NOT VALID;
ALTER TABLE "broadcast_recipients" VALIDATE CONSTRAINT "broadcast_recipients_broadcasts_recipients";
ALTER TABLE "broadcast_recipients" ADD CONSTRAINT "broadcast_recipients_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "broadcast_recipients" VALIDATE CONSTRAINT "broadcast_recipients_contacts_contact";
-- modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_integrations_integration";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_segments_segment";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_integrations_integration" FOREIGN KEY ("workspace_id", "integration_id") REFERENCES "integrations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_integrations_integration";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_segments_segment" FOREIGN KEY ("workspace_id", "segment_id") REFERENCES "segments" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_segments_segment";
-- modify "captured_messages" table
ALTER TABLE "captured_messages" DROP CONSTRAINT "captured_messages_integrations_captured_messages";
ALTER TABLE "captured_messages" DROP CONSTRAINT "captured_messages_outbound_messages_outbound_message";
ALTER TABLE "captured_messages" ADD CONSTRAINT "captured_messages_integrations_captured_messages" FOREIGN KEY ("workspace_id", "integration_id") REFERENCES "integrations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "captured_messages" VALIDATE CONSTRAINT "captured_messages_integrations_captured_messages";
ALTER TABLE "captured_messages" ADD CONSTRAINT "captured_messages_outbound_messages_outbound_message" FOREIGN KEY ("workspace_id", "outbound_message_id") REFERENCES "outbound_messages" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (outbound_message_id) NOT VALID;
ALTER TABLE "captured_messages" VALIDATE CONSTRAINT "captured_messages_outbound_messages_outbound_message";
-- modify "confirmations" table
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_contacts_contact";
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_contacts_contact";
-- modify "events" table
ALTER TABLE "events" DROP CONSTRAINT "events_contacts_events";
ALTER TABLE "events" ADD CONSTRAINT "events_contacts_events" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "events" VALIDATE CONSTRAINT "events_contacts_events";
-- modify "oauth_codes" table
ALTER TABLE "oauth_codes" DROP CONSTRAINT "oauth_codes_memberships_oauth_codes";
ALTER TABLE "oauth_codes" ADD CONSTRAINT "oauth_codes_memberships_oauth_codes" FOREIGN KEY ("workspace_id", "membership_id") REFERENCES "memberships" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "oauth_codes" VALIDATE CONSTRAINT "oauth_codes_memberships_oauth_codes";
-- modify "order_movements" table
ALTER TABLE "order_movements" DROP CONSTRAINT "order_movements_orders_movements";
ALTER TABLE "order_movements" ADD CONSTRAINT "order_movements_orders_movements" FOREIGN KEY ("workspace_id", "order_id") REFERENCES "orders" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "order_movements" VALIDATE CONSTRAINT "order_movements_orders_movements";
-- modify "orders" table
ALTER TABLE "orders" DROP CONSTRAINT "orders_automations_automation";
ALTER TABLE "orders" DROP CONSTRAINT "orders_broadcasts_broadcast";
ALTER TABLE "orders" DROP CONSTRAINT "orders_contacts_orders";
ALTER TABLE "orders" ADD CONSTRAINT "orders_automations_automation" FOREIGN KEY ("workspace_id", "automation_id") REFERENCES "automations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "orders" VALIDATE CONSTRAINT "orders_automations_automation";
ALTER TABLE "orders" ADD CONSTRAINT "orders_broadcasts_broadcast" FOREIGN KEY ("workspace_id", "broadcast_id") REFERENCES "broadcasts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "orders" VALIDATE CONSTRAINT "orders_broadcasts_broadcast";
ALTER TABLE "orders" ADD CONSTRAINT "orders_contacts_orders" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "orders" VALIDATE CONSTRAINT "orders_contacts_orders";
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_automation_runs_automation_run";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_automations_automation";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_broadcasts_broadcast";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_contacts_contact";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_email_templates_template";
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_integrations_integration";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_automation_runs_automation_run" FOREIGN KEY ("workspace_id", "automation_run_id") REFERENCES "automation_runs" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (automation_run_id) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_automation_runs_automation_run";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_automations_automation" FOREIGN KEY ("workspace_id", "automation_id") REFERENCES "automations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_automations_automation";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient" FOREIGN KEY ("workspace_id", "broadcast_recipient_id") REFERENCES "broadcast_recipients" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (broadcast_recipient_id) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_broadcasts_broadcast" FOREIGN KEY ("workspace_id", "broadcast_id") REFERENCES "broadcasts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_broadcasts_broadcast";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_contacts_contact";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_email_templates_template" FOREIGN KEY ("workspace_id", "template_id") REFERENCES "email_templates" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (template_id) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_email_templates_template";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_integrations_integration" FOREIGN KEY ("workspace_id", "integration_id") REFERENCES "integrations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_integrations_integration";
-- modify "provider_limiters" table
ALTER TABLE "provider_limiters" DROP CONSTRAINT "provider_limiters_integrations_provider_limiter";
ALTER TABLE "provider_limiters" ADD CONSTRAINT "provider_limiters_integrations_provider_limiter" FOREIGN KEY ("workspace_id", "integration_id") REFERENCES "integrations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "provider_limiters" VALIDATE CONSTRAINT "provider_limiters_integrations_provider_limiter";
-- modify "rule_firings" table
ALTER TABLE "rule_firings" DROP CONSTRAINT "rule_firings_contacts_contact";
ALTER TABLE "rule_firings" DROP CONSTRAINT "rule_firings_rule_versions_firings";
ALTER TABLE "rule_firings" DROP CONSTRAINT "rule_firings_rules_firings";
ALTER TABLE "rule_firings" ADD CONSTRAINT "rule_firings_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "rule_firings" VALIDATE CONSTRAINT "rule_firings_contacts_contact";
ALTER TABLE "rule_firings" ADD CONSTRAINT "rule_firings_rule_versions_firings" FOREIGN KEY ("workspace_id", "version_id") REFERENCES "rule_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE NO ACTION NOT VALID;
ALTER TABLE "rule_firings" VALIDATE CONSTRAINT "rule_firings_rule_versions_firings";
ALTER TABLE "rule_firings" ADD CONSTRAINT "rule_firings_rules_firings" FOREIGN KEY ("workspace_id", "rule_id") REFERENCES "rules" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "rule_firings" VALIDATE CONSTRAINT "rule_firings_rules_firings";
-- modify "rule_version_triggers" table
ALTER TABLE "rule_version_triggers" DROP CONSTRAINT "rule_version_triggers_rule_versions_triggers";
ALTER TABLE "rule_version_triggers" ADD CONSTRAINT "rule_version_triggers_rule_versions_triggers" FOREIGN KEY ("workspace_id", "version_id") REFERENCES "rule_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "rule_version_triggers" VALIDATE CONSTRAINT "rule_version_triggers_rule_versions_triggers";
-- modify "rule_versions" table
ALTER TABLE "rule_versions" DROP CONSTRAINT "rule_versions_rules_versions";
ALTER TABLE "rule_versions" ADD CONSTRAINT "rule_versions_rules_versions" FOREIGN KEY ("workspace_id", "rule_id") REFERENCES "rules" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "rule_versions" VALIDATE CONSTRAINT "rule_versions_rules_versions";
-- modify "rules" table
ALTER TABLE "rules" DROP CONSTRAINT "rules_rule_versions_draft_version";
ALTER TABLE "rules" DROP CONSTRAINT "rules_rule_versions_live_version";
ALTER TABLE "rules" ADD CONSTRAINT "rules_rule_versions_draft_version" FOREIGN KEY ("workspace_id", "draft_version_id") REFERENCES "rule_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (draft_version_id) NOT VALID;
ALTER TABLE "rules" VALIDATE CONSTRAINT "rules_rule_versions_draft_version";
ALTER TABLE "rules" ADD CONSTRAINT "rules_rule_versions_live_version" FOREIGN KEY ("workspace_id", "live_version_id") REFERENCES "rule_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (live_version_id) NOT VALID;
ALTER TABLE "rules" VALIDATE CONSTRAINT "rules_rule_versions_live_version";
-- modify "start_tokens" table
ALTER TABLE "start_tokens" DROP CONSTRAINT "start_tokens_contacts_start_tokens";
ALTER TABLE "start_tokens" DROP CONSTRAINT "start_tokens_integrations_start_tokens";
ALTER TABLE "start_tokens" DROP CONSTRAINT "start_tokens_visitors_start_tokens";
ALTER TABLE "start_tokens" ADD CONSTRAINT "start_tokens_contacts_start_tokens" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "start_tokens" VALIDATE CONSTRAINT "start_tokens_contacts_start_tokens";
ALTER TABLE "start_tokens" ADD CONSTRAINT "start_tokens_integrations_start_tokens" FOREIGN KEY ("workspace_id", "integration_id") REFERENCES "integrations" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "start_tokens" VALIDATE CONSTRAINT "start_tokens_integrations_start_tokens";
ALTER TABLE "start_tokens" ADD CONSTRAINT "start_tokens_visitors_start_tokens" FOREIGN KEY ("workspace_id", "visitor_id") REFERENCES "visitors" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "start_tokens" VALIDATE CONSTRAINT "start_tokens_visitors_start_tokens";
-- modify "suppressions" table
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_contacts_contact";
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_contacts_contact";
-- modify "tracked_links" table
ALTER TABLE "tracked_links" DROP CONSTRAINT "tracked_links_outbound_messages_tracked_links";
ALTER TABLE "tracked_links" ADD CONSTRAINT "tracked_links_outbound_messages_tracked_links" FOREIGN KEY ("workspace_id", "outbound_message_id") REFERENCES "outbound_messages" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "tracked_links" VALIDATE CONSTRAINT "tracked_links_outbound_messages_tracked_links";
-- modify "unsubscribes" table
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_contacts_contact";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_contacts_contact" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_contacts_contact";
-- modify "visitors" table
ALTER TABLE "visitors" DROP CONSTRAINT "visitors_contacts_visitors";
ALTER TABLE "visitors" ADD CONSTRAINT "visitors_contacts_visitors" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE SET NULL (contact_id) NOT VALID;
ALTER TABLE "visitors" VALIDATE CONSTRAINT "visitors_contacts_visitors";
DROP INDEX CONCURRENTLY "apitoken_membership_id";
DROP INDEX CONCURRENTLY "apitoken_workspace_id";
DROP INDEX CONCURRENTLY "automation_runs_automation_id_contact_id";
DROP INDEX CONCURRENTLY "automationrun_version_id";
DROP INDEX CONCURRENTLY "automation_draft_version_id";
DROP INDEX CONCURRENTLY "automation_live_version_id";
DROP INDEX CONCURRENTLY "automation_workspace_id";
DROP INDEX CONCURRENTLY "binding_workspace_id";
DROP INDEX CONCURRENTLY "bindings_contact_id";
DROP INDEX CONCURRENTLY "bindings_integration_destination";
DROP INDEX CONCURRENTLY "bindings_visitor_id";
DROP INDEX CONCURRENTLY "broadcast_recipients_broadcast_id_contact_id";
DROP INDEX CONCURRENTLY "broadcast_recipients_contact_id";
DROP INDEX CONCURRENTLY "broadcastrecipient_workspace_id";
DROP INDEX CONCURRENTLY "broadcast_integration_id";
DROP INDEX CONCURRENTLY "broadcast_segment_id";
DROP INDEX CONCURRENTLY "broadcast_workspace_id";
DROP INDEX CONCURRENTLY "captured_messages_integration_id";
DROP INDEX CONCURRENTLY "captured_messages_outbound_message_id";
DROP INDEX CONCURRENTLY "confirmations_contact_id";
DROP INDEX CONCURRENTLY "emailtemplate_workspace_id";
DROP INDEX CONCURRENTLY "integration_workspace_id";
DROP INDEX CONCURRENTLY "membership_workspace_id";
DROP INDEX CONCURRENTLY "oauthcode_membership_id";
DROP INDEX CONCURRENTLY "oauthcode_workspace_id";
DROP INDEX CONCURRENTLY "ordermovement_order_id";
DROP INDEX CONCURRENTLY "order_automation_id";
DROP INDEX CONCURRENTLY "order_broadcast_id";
DROP INDEX CONCURRENTLY "order_contact_id";
DROP INDEX CONCURRENTLY "outbound_messages_automation_id";
DROP INDEX CONCURRENTLY "outbound_messages_automation_run_id";
DROP INDEX CONCURRENTLY "outbound_messages_broadcast_recipient_id";
DROP INDEX CONCURRENTLY "outbound_messages_contact_id";
DROP INDEX CONCURRENTLY "outbound_messages_template_id";
DROP INDEX CONCURRENTLY "providerlimiter_workspace_id";
DROP INDEX CONCURRENTLY "rule_firings_contact_id";
DROP INDEX CONCURRENTLY "rulefiring_rule_id_source_id";
DROP INDEX CONCURRENTLY "rulefiring_version_id";
DROP INDEX CONCURRENTLY "rulefiring_workspace_id";
DROP INDEX CONCURRENTLY "ruleversiontrigger_version_id_action";
DROP INDEX CONCURRENTLY "rule_draft_version_id";
DROP INDEX CONCURRENTLY "rule_live_version_id";
DROP INDEX CONCURRENTLY "rule_workspace_id";
DROP INDEX CONCURRENTLY "segment_workspace_id";
DROP INDEX CONCURRENTLY "start_tokens_contact_id";
DROP INDEX CONCURRENTLY "start_tokens_integration_id";
DROP INDEX CONCURRENTLY "start_tokens_visitor_id";
DROP INDEX CONCURRENTLY "tracked_links_outbound_message_id";
DROP INDEX CONCURRENTLY "trackedlink_workspace_id";
DROP INDEX CONCURRENTLY "unsubscribes_contact_id";
DROP INDEX CONCURRENTLY "visitor_contact_id";

-- +goose Down
-- reverse: create index "visitors_workspace_id_id" to table: "visitors"
DROP INDEX "visitors_workspace_id_id";
-- reverse: create index "visitor_workspace_id_contact_id" to table: "visitors"
DROP INDEX "visitor_workspace_id_contact_id";
-- reverse: modify "visitors" table
ALTER TABLE "visitors" DROP CONSTRAINT "visitors_contacts_visitors", ADD CONSTRAINT "visitors_contacts_visitors" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: drop index "visitor_contact_id" from table: "visitors"
CREATE INDEX "visitor_contact_id" ON "visitors" ("contact_id");
-- reverse: create index "unsubscribes_workspace_id_contact_id" to table: "unsubscribes"
DROP INDEX "unsubscribes_workspace_id_contact_id";
-- reverse: modify "unsubscribes" table
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_contacts_contact", ADD CONSTRAINT "unsubscribes_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: drop index "unsubscribes_contact_id" from table: "unsubscribes"
CREATE INDEX "unsubscribes_contact_id" ON "unsubscribes" ("contact_id");
-- reverse: create index "tracked_links_workspace_id_outbound_message_id" to table: "tracked_links"
DROP INDEX "tracked_links_workspace_id_outbound_message_id";
-- reverse: modify "tracked_links" table
ALTER TABLE "tracked_links" DROP CONSTRAINT "tracked_links_outbound_messages_tracked_links", ADD CONSTRAINT "tracked_links_outbound_messages_tracked_links" FOREIGN KEY ("outbound_message_id") REFERENCES "outbound_messages" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "trackedlink_workspace_id" from table: "tracked_links"
CREATE INDEX "trackedlink_workspace_id" ON "tracked_links" ("workspace_id");
-- reverse: drop index "tracked_links_outbound_message_id" from table: "tracked_links"
CREATE INDEX "tracked_links_outbound_message_id" ON "tracked_links" ("outbound_message_id");
-- reverse: modify "suppressions" table
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_contacts_contact", ADD CONSTRAINT "suppressions_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: create index "start_tokens_workspace_id_visitor_id" to table: "start_tokens"
DROP INDEX "start_tokens_workspace_id_visitor_id";
-- reverse: create index "start_tokens_workspace_id_integration_id" to table: "start_tokens"
DROP INDEX "start_tokens_workspace_id_integration_id";
-- reverse: create index "start_tokens_workspace_id_contact_id" to table: "start_tokens"
DROP INDEX "start_tokens_workspace_id_contact_id";
-- reverse: modify "start_tokens" table
ALTER TABLE "start_tokens" DROP CONSTRAINT "start_tokens_visitors_start_tokens", DROP CONSTRAINT "start_tokens_integrations_start_tokens", DROP CONSTRAINT "start_tokens_contacts_start_tokens", ADD CONSTRAINT "start_tokens_visitors_start_tokens" FOREIGN KEY ("visitor_id") REFERENCES "visitors" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "start_tokens_integrations_start_tokens" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "start_tokens_contacts_start_tokens" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "start_tokens_visitor_id" from table: "start_tokens"
CREATE INDEX "start_tokens_visitor_id" ON "start_tokens" ("visitor_id");
-- reverse: drop index "start_tokens_integration_id" from table: "start_tokens"
CREATE INDEX "start_tokens_integration_id" ON "start_tokens" ("integration_id");
-- reverse: drop index "start_tokens_contact_id" from table: "start_tokens"
CREATE INDEX "start_tokens_contact_id" ON "start_tokens" ("contact_id");
-- reverse: create index "segments_workspace_id_id" to table: "segments"
DROP INDEX "segments_workspace_id_id";
-- reverse: drop index "segment_workspace_id" from table: "segments"
CREATE INDEX "segment_workspace_id" ON "segments" ("workspace_id");
-- reverse: create index "rules_workspace_id_id" to table: "rules"
DROP INDEX "rules_workspace_id_id";
-- reverse: create index "rule_workspace_id_live_version_id" to table: "rules"
DROP INDEX "rule_workspace_id_live_version_id";
-- reverse: create index "rule_workspace_id_draft_version_id" to table: "rules"
DROP INDEX "rule_workspace_id_draft_version_id";
-- reverse: modify "rules" table
ALTER TABLE "rules" DROP CONSTRAINT "rules_rule_versions_live_version", DROP CONSTRAINT "rules_rule_versions_draft_version", ADD CONSTRAINT "rules_rule_versions_live_version" FOREIGN KEY ("live_version_id") REFERENCES "rule_versions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "rules_rule_versions_draft_version" FOREIGN KEY ("draft_version_id") REFERENCES "rule_versions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: drop index "rule_workspace_id" from table: "rules"
CREATE INDEX "rule_workspace_id" ON "rules" ("workspace_id");
-- reverse: drop index "rule_live_version_id" from table: "rules"
CREATE INDEX "rule_live_version_id" ON "rules" ("live_version_id");
-- reverse: drop index "rule_draft_version_id" from table: "rules"
CREATE INDEX "rule_draft_version_id" ON "rules" ("draft_version_id");
-- reverse: create index "rule_versions_workspace_id_id" to table: "rule_versions"
DROP INDEX "rule_versions_workspace_id_id";
-- reverse: modify "rule_versions" table
ALTER TABLE "rule_versions" DROP CONSTRAINT "rule_versions_rules_versions", ADD CONSTRAINT "rule_versions_rules_versions" FOREIGN KEY ("rule_id") REFERENCES "rules" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: create index "ruleversiontrigger_workspace_id_version_id_action" to table: "rule_version_triggers"
DROP INDEX "ruleversiontrigger_workspace_id_version_id_action";
-- reverse: modify "rule_version_triggers" table
ALTER TABLE "rule_version_triggers" DROP CONSTRAINT "rule_version_triggers_rule_versions_triggers", ADD CONSTRAINT "rule_version_triggers_rule_versions_triggers" FOREIGN KEY ("version_id") REFERENCES "rule_versions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "ruleversiontrigger_version_id_action" from table: "rule_version_triggers"
CREATE UNIQUE INDEX "ruleversiontrigger_version_id_action" ON "rule_version_triggers" ("version_id", "action");
-- reverse: create index "rulefiring_workspace_id_version_id" to table: "rule_firings"
DROP INDEX "rulefiring_workspace_id_version_id";
-- reverse: create index "rulefiring_workspace_id_rule_id_source_id" to table: "rule_firings"
DROP INDEX "rulefiring_workspace_id_rule_id_source_id";
-- reverse: create index "rule_firings_workspace_id_contact_id" to table: "rule_firings"
DROP INDEX "rule_firings_workspace_id_contact_id";
-- reverse: modify "rule_firings" table
ALTER TABLE "rule_firings" DROP CONSTRAINT "rule_firings_rules_firings", DROP CONSTRAINT "rule_firings_rule_versions_firings", DROP CONSTRAINT "rule_firings_contacts_contact", ADD CONSTRAINT "rule_firings_rules_firings" FOREIGN KEY ("rule_id") REFERENCES "rules" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "rule_firings_rule_versions_firings" FOREIGN KEY ("version_id") REFERENCES "rule_versions" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, ADD CONSTRAINT "rule_firings_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "rulefiring_workspace_id" from table: "rule_firings"
CREATE INDEX "rulefiring_workspace_id" ON "rule_firings" ("workspace_id");
-- reverse: drop index "rulefiring_version_id" from table: "rule_firings"
CREATE INDEX "rulefiring_version_id" ON "rule_firings" ("version_id");
-- reverse: drop index "rulefiring_rule_id_source_id" from table: "rule_firings"
CREATE UNIQUE INDEX "rulefiring_rule_id_source_id" ON "rule_firings" ("rule_id", "source_id");
-- reverse: drop index "rule_firings_contact_id" from table: "rule_firings"
CREATE INDEX "rule_firings_contact_id" ON "rule_firings" ("contact_id");
-- reverse: create index "providerlimiter_workspace_id_integration_id" to table: "provider_limiters"
DROP INDEX "providerlimiter_workspace_id_integration_id";
-- reverse: modify "provider_limiters" table
ALTER TABLE "provider_limiters" DROP CONSTRAINT "provider_limiters_integrations_provider_limiter", ADD CONSTRAINT "provider_limiters_integrations_provider_limiter" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "providerlimiter_workspace_id" from table: "provider_limiters"
CREATE INDEX "providerlimiter_workspace_id" ON "provider_limiters" ("workspace_id");
-- reverse: create index "outbound_messages_workspace_id_template_id" to table: "outbound_messages"
DROP INDEX "outbound_messages_workspace_id_template_id";
-- reverse: create index "outbound_messages_workspace_id_id" to table: "outbound_messages"
DROP INDEX "outbound_messages_workspace_id_id";
-- reverse: create index "outbound_messages_workspace_id_contact_id" to table: "outbound_messages"
DROP INDEX "outbound_messages_workspace_id_contact_id";
-- reverse: create index "outbound_messages_workspace_id_broadcast_recipient_id" to table: "outbound_messages"
DROP INDEX "outbound_messages_workspace_id_broadcast_recipient_id";
-- reverse: create index "outbound_messages_workspace_id_automation_run_id" to table: "outbound_messages"
DROP INDEX "outbound_messages_workspace_id_automation_run_id";
-- reverse: create index "outbound_messages_workspace_id_automation_id" to table: "outbound_messages"
DROP INDEX "outbound_messages_workspace_id_automation_id";
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_integrations_integration", DROP CONSTRAINT "outbound_messages_email_templates_template", DROP CONSTRAINT "outbound_messages_contacts_contact", DROP CONSTRAINT "outbound_messages_broadcasts_broadcast", DROP CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient", DROP CONSTRAINT "outbound_messages_automations_automation", DROP CONSTRAINT "outbound_messages_automation_runs_automation_run", ADD CONSTRAINT "outbound_messages_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT, ADD CONSTRAINT "outbound_messages_email_templates_template" FOREIGN KEY ("template_id") REFERENCES "email_templates" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "outbound_messages_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "outbound_messages_broadcasts_broadcast" FOREIGN KEY ("broadcast_id") REFERENCES "broadcasts" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT, ADD CONSTRAINT "outbound_messages_broadcast_recipients_broadcast_recipient" FOREIGN KEY ("broadcast_recipient_id") REFERENCES "broadcast_recipients" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "outbound_messages_automations_automation" FOREIGN KEY ("automation_id") REFERENCES "automations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT, ADD CONSTRAINT "outbound_messages_automation_runs_automation_run" FOREIGN KEY ("automation_run_id") REFERENCES "automation_runs" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: drop index "outbound_messages_template_id" from table: "outbound_messages"
CREATE INDEX "outbound_messages_template_id" ON "outbound_messages" ("template_id");
-- reverse: drop index "outbound_messages_contact_id" from table: "outbound_messages"
CREATE INDEX "outbound_messages_contact_id" ON "outbound_messages" ("contact_id");
-- reverse: drop index "outbound_messages_broadcast_recipient_id" from table: "outbound_messages"
CREATE INDEX "outbound_messages_broadcast_recipient_id" ON "outbound_messages" ("broadcast_recipient_id");
-- reverse: drop index "outbound_messages_automation_run_id" from table: "outbound_messages"
CREATE INDEX "outbound_messages_automation_run_id" ON "outbound_messages" ("automation_run_id");
-- reverse: drop index "outbound_messages_automation_id" from table: "outbound_messages"
CREATE INDEX "outbound_messages_automation_id" ON "outbound_messages" ("automation_id");
-- reverse: create index "orders_workspace_id_id" to table: "orders"
DROP INDEX "orders_workspace_id_id";
-- reverse: create index "order_workspace_id_contact_id" to table: "orders"
DROP INDEX "order_workspace_id_contact_id";
-- reverse: create index "order_workspace_id_broadcast_id" to table: "orders"
DROP INDEX "order_workspace_id_broadcast_id";
-- reverse: create index "order_workspace_id_automation_id" to table: "orders"
DROP INDEX "order_workspace_id_automation_id";
-- reverse: modify "orders" table
ALTER TABLE "orders" DROP CONSTRAINT "orders_contacts_orders", DROP CONSTRAINT "orders_broadcasts_broadcast", DROP CONSTRAINT "orders_automations_automation", ADD CONSTRAINT "orders_contacts_orders" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "orders_broadcasts_broadcast" FOREIGN KEY ("broadcast_id") REFERENCES "broadcasts" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT, ADD CONSTRAINT "orders_automations_automation" FOREIGN KEY ("automation_id") REFERENCES "automations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT;
-- reverse: drop index "order_contact_id" from table: "orders"
CREATE INDEX "order_contact_id" ON "orders" ("contact_id");
-- reverse: drop index "order_broadcast_id" from table: "orders"
CREATE INDEX "order_broadcast_id" ON "orders" ("broadcast_id");
-- reverse: drop index "order_automation_id" from table: "orders"
CREATE INDEX "order_automation_id" ON "orders" ("automation_id");
-- reverse: create index "ordermovement_workspace_id_order_id" to table: "order_movements"
DROP INDEX "ordermovement_workspace_id_order_id";
-- reverse: modify "order_movements" table
ALTER TABLE "order_movements" DROP CONSTRAINT "order_movements_orders_movements", ADD CONSTRAINT "order_movements_orders_movements" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "ordermovement_order_id" from table: "order_movements"
CREATE INDEX "ordermovement_order_id" ON "order_movements" ("order_id");
-- reverse: create index "oauthcode_workspace_id_membership_id" to table: "oauth_codes"
DROP INDEX "oauthcode_workspace_id_membership_id";
-- reverse: modify "oauth_codes" table
ALTER TABLE "oauth_codes" DROP CONSTRAINT "oauth_codes_memberships_oauth_codes", ADD CONSTRAINT "oauth_codes_memberships_oauth_codes" FOREIGN KEY ("membership_id") REFERENCES "memberships" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "oauthcode_workspace_id" from table: "oauth_codes"
CREATE INDEX "oauthcode_workspace_id" ON "oauth_codes" ("workspace_id");
-- reverse: drop index "oauthcode_membership_id" from table: "oauth_codes"
CREATE INDEX "oauthcode_membership_id" ON "oauth_codes" ("membership_id");
-- reverse: create index "memberships_workspace_id_id" to table: "memberships"
DROP INDEX "memberships_workspace_id_id";
-- reverse: drop index "membership_workspace_id" from table: "memberships"
CREATE INDEX "membership_workspace_id" ON "memberships" ("workspace_id");
-- reverse: create index "integrations_workspace_id_id" to table: "integrations"
DROP INDEX "integrations_workspace_id_id";
-- reverse: drop index "integration_workspace_id" from table: "integrations"
CREATE INDEX "integration_workspace_id" ON "integrations" ("workspace_id");
-- reverse: modify "events" table
ALTER TABLE "events" DROP CONSTRAINT "events_contacts_events", ADD CONSTRAINT "events_contacts_events" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: create index "email_templates_workspace_id_id" to table: "email_templates"
DROP INDEX "email_templates_workspace_id_id";
-- reverse: drop index "emailtemplate_workspace_id" from table: "email_templates"
CREATE INDEX "emailtemplate_workspace_id" ON "email_templates" ("workspace_id");
-- reverse: create index "contacts_workspace_id_id" to table: "contacts"
DROP INDEX "contacts_workspace_id_id";
-- reverse: create index "confirmations_workspace_id_contact_id" to table: "confirmations"
DROP INDEX "confirmations_workspace_id_contact_id";
-- reverse: modify "confirmations" table
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_contacts_contact", ADD CONSTRAINT "confirmations_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "confirmations_contact_id" from table: "confirmations"
CREATE INDEX "confirmations_contact_id" ON "confirmations" ("contact_id");
-- reverse: create index "captured_messages_workspace_id_outbound_message_id" to table: "captured_messages"
DROP INDEX "captured_messages_workspace_id_outbound_message_id";
-- reverse: create index "captured_messages_workspace_id_integration_id" to table: "captured_messages"
DROP INDEX "captured_messages_workspace_id_integration_id";
-- reverse: modify "captured_messages" table
ALTER TABLE "captured_messages" DROP CONSTRAINT "captured_messages_outbound_messages_outbound_message", DROP CONSTRAINT "captured_messages_integrations_captured_messages", ADD CONSTRAINT "captured_messages_outbound_messages_outbound_message" FOREIGN KEY ("outbound_message_id") REFERENCES "outbound_messages" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "captured_messages_integrations_captured_messages" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "captured_messages_outbound_message_id" from table: "captured_messages"
CREATE INDEX "captured_messages_outbound_message_id" ON "captured_messages" ("outbound_message_id");
-- reverse: drop index "captured_messages_integration_id" from table: "captured_messages"
CREATE INDEX "captured_messages_integration_id" ON "captured_messages" ("integration_id");
-- reverse: create index "broadcasts_workspace_id_id" to table: "broadcasts"
DROP INDEX "broadcasts_workspace_id_id";
-- reverse: create index "broadcast_workspace_id_segment_id" to table: "broadcasts"
DROP INDEX "broadcast_workspace_id_segment_id";
-- reverse: create index "broadcast_workspace_id_integration_id" to table: "broadcasts"
DROP INDEX "broadcast_workspace_id_integration_id";
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_segments_segment", DROP CONSTRAINT "broadcasts_integrations_integration", ADD CONSTRAINT "broadcasts_segments_segment" FOREIGN KEY ("segment_id") REFERENCES "segments" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT, ADD CONSTRAINT "broadcasts_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT;
-- reverse: drop index "broadcast_workspace_id" from table: "broadcasts"
CREATE INDEX "broadcast_workspace_id" ON "broadcasts" ("workspace_id");
-- reverse: drop index "broadcast_segment_id" from table: "broadcasts"
CREATE INDEX "broadcast_segment_id" ON "broadcasts" ("segment_id");
-- reverse: drop index "broadcast_integration_id" from table: "broadcasts"
CREATE INDEX "broadcast_integration_id" ON "broadcasts" ("integration_id");
-- reverse: create index "broadcast_recipients_workspace_id_id" to table: "broadcast_recipients"
DROP INDEX "broadcast_recipients_workspace_id_id";
-- reverse: create index "broadcast_recipients_workspace_id_contact_id" to table: "broadcast_recipients"
DROP INDEX "broadcast_recipients_workspace_id_contact_id";
-- reverse: create index "broadcast_recipients_workspace_id_broadcast_id_contact_id" to table: "broadcast_recipients"
DROP INDEX "broadcast_recipients_workspace_id_broadcast_id_contact_id";
-- reverse: modify "broadcast_recipients" table
ALTER TABLE "broadcast_recipients" DROP CONSTRAINT "broadcast_recipients_contacts_contact", DROP CONSTRAINT "broadcast_recipients_broadcasts_recipients", ADD CONSTRAINT "broadcast_recipients_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "broadcast_recipients_broadcasts_recipients" FOREIGN KEY ("broadcast_id") REFERENCES "broadcasts" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
-- reverse: drop index "broadcastrecipient_workspace_id" from table: "broadcast_recipients"
CREATE INDEX "broadcastrecipient_workspace_id" ON "broadcast_recipients" ("workspace_id");
-- reverse: drop index "broadcast_recipients_contact_id" from table: "broadcast_recipients"
CREATE INDEX "broadcast_recipients_contact_id" ON "broadcast_recipients" ("contact_id");
-- reverse: drop index "broadcast_recipients_broadcast_id_contact_id" from table: "broadcast_recipients"
CREATE UNIQUE INDEX "broadcast_recipients_broadcast_id_contact_id" ON "broadcast_recipients" ("broadcast_id", "contact_id");
-- reverse: create index "bindings_workspace_id_visitor_id" to table: "bindings"
DROP INDEX "bindings_workspace_id_visitor_id";
-- reverse: create index "bindings_workspace_id_integration_destination" to table: "bindings"
DROP INDEX "bindings_workspace_id_integration_destination";
-- reverse: create index "bindings_workspace_id_contact_id" to table: "bindings"
DROP INDEX "bindings_workspace_id_contact_id";
-- reverse: modify "bindings" table
ALTER TABLE "bindings" DROP CONSTRAINT "bindings_visitors_bindings", DROP CONSTRAINT "bindings_integrations_bindings", DROP CONSTRAINT "bindings_contacts_bindings", ADD CONSTRAINT "bindings_visitors_bindings" FOREIGN KEY ("visitor_id") REFERENCES "visitors" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "bindings_integrations_bindings" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "bindings_contacts_bindings" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "bindings_visitor_id" from table: "bindings"
CREATE INDEX "bindings_visitor_id" ON "bindings" ("visitor_id");
-- reverse: drop index "bindings_integration_destination" from table: "bindings"
CREATE UNIQUE INDEX "bindings_integration_destination" ON "bindings" ("integration_id", "destination");
-- reverse: drop index "bindings_contact_id" from table: "bindings"
CREATE INDEX "bindings_contact_id" ON "bindings" ("contact_id");
-- reverse: drop index "binding_workspace_id" from table: "bindings"
CREATE INDEX "binding_workspace_id" ON "bindings" ("workspace_id");
-- reverse: create index "automations_workspace_id_id" to table: "automations"
DROP INDEX "automations_workspace_id_id";
-- reverse: create index "automation_workspace_id_live_version_id" to table: "automations"
DROP INDEX "automation_workspace_id_live_version_id";
-- reverse: create index "automation_workspace_id_draft_version_id" to table: "automations"
DROP INDEX "automation_workspace_id_draft_version_id";
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP CONSTRAINT "automations_automation_versions_live_version", DROP CONSTRAINT "automations_automation_versions_draft_version", ADD CONSTRAINT "automations_automation_versions_live_version" FOREIGN KEY ("live_version_id") REFERENCES "automation_versions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, ADD CONSTRAINT "automations_automation_versions_draft_version" FOREIGN KEY ("draft_version_id") REFERENCES "automation_versions" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- reverse: drop index "automation_workspace_id" from table: "automations"
CREATE INDEX "automation_workspace_id" ON "automations" ("workspace_id");
-- reverse: drop index "automation_live_version_id" from table: "automations"
CREATE INDEX "automation_live_version_id" ON "automations" ("live_version_id");
-- reverse: drop index "automation_draft_version_id" from table: "automations"
CREATE INDEX "automation_draft_version_id" ON "automations" ("draft_version_id");
-- reverse: create index "automation_versions_workspace_id_id" to table: "automation_versions"
DROP INDEX "automation_versions_workspace_id_id";
-- reverse: modify "automation_versions" table
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_automations_versions", ADD CONSTRAINT "automation_versions_automations_versions" FOREIGN KEY ("automation_id") REFERENCES "automations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: create index "automationrun_workspace_id_version_id" to table: "automation_runs"
DROP INDEX "automationrun_workspace_id_version_id";
-- reverse: create index "automation_runs_workspace_id_id" to table: "automation_runs"
DROP INDEX "automation_runs_workspace_id_id";
-- reverse: create index "automation_runs_workspace_id_automation_id_contact_id" to table: "automation_runs"
DROP INDEX "automation_runs_workspace_id_automation_id_contact_id";
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_contacts_contact", DROP CONSTRAINT "automation_runs_automations_runs", DROP CONSTRAINT "automation_runs_automation_versions_runs", ADD CONSTRAINT "automation_runs_contacts_contact" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "automation_runs_automations_runs" FOREIGN KEY ("automation_id") REFERENCES "automations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "automation_runs_automation_versions_runs" FOREIGN KEY ("version_id") REFERENCES "automation_versions" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
-- reverse: drop index "automationrun_version_id" from table: "automation_runs"
CREATE INDEX "automationrun_version_id" ON "automation_runs" ("version_id");
-- reverse: drop index "automation_runs_automation_id_contact_id" from table: "automation_runs"
CREATE UNIQUE INDEX "automation_runs_automation_id_contact_id" ON "automation_runs" ("automation_id", "contact_id");
-- reverse: create index "apitoken_workspace_id_membership_id" to table: "api_tokens"
DROP INDEX "apitoken_workspace_id_membership_id";
-- reverse: modify "api_tokens" table
ALTER TABLE "api_tokens" DROP CONSTRAINT "api_tokens_memberships_connections", ADD CONSTRAINT "api_tokens_memberships_connections" FOREIGN KEY ("membership_id") REFERENCES "memberships" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
-- reverse: drop index "apitoken_workspace_id" from table: "api_tokens"
CREATE INDEX "apitoken_workspace_id" ON "api_tokens" ("workspace_id");
-- reverse: drop index "apitoken_membership_id" from table: "api_tokens"
CREATE INDEX "apitoken_membership_id" ON "api_tokens" ("membership_id");
