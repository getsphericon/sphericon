-- +goose NO TRANSACTION
-- +goose Up
-- A table that keeps several kinds of row, or a value the owner checks, now has the database check it
-- too: an Outbound message carries only its kind's provenance, an Order at most one credit, an
-- Integration its provider's channel, an Audit entry a known actor kind, a currency its ISO 4217 form.
-- Every writer already keeps these rules, so each CHECK is added NOT VALID and validated after,
-- and the table is not locked while it is checked.
-- modify "audit_entries" table
ALTER TABLE "audit_entries" ADD CONSTRAINT "audit_entries_actor_kind_check" CHECK ((actor_kind)::text = ANY ((ARRAY['user'::character varying, 'api_token'::character varying, 'staff'::character varying, 'service'::character varying, 'system'::character varying])::text[])) NOT VALID;
ALTER TABLE "audit_entries" VALIDATE CONSTRAINT "audit_entries_actor_kind_check";
-- modify "integrations" table
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_provider_channel_check" CHECK ((((channel)::text = 'email'::text) AND ((provider)::text = ANY ((ARRAY['smtp'::character varying, 'ses'::character varying, 'inbox'::character varying])::text[]))) OR (((channel)::text = 'telegram'::text) AND ((provider)::text = 'telegram'::text)) OR (((channel)::text = 'sms'::text) AND ((provider)::text = 'twilio'::text))) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_channel_check";
ALTER TABLE "integrations" ALTER COLUMN "channel" DROP DEFAULT;
-- modify "orders" table
ALTER TABLE "orders" ADD CONSTRAINT "orders_credit_check" CHECK (((broadcast_id IS NULL) OR (automation_id IS NULL)) AND ((credited_click_at IS NULL) = ((broadcast_id IS NULL) AND (automation_id IS NULL)))) NOT VALID;
ALTER TABLE "orders" VALIDATE CONSTRAINT "orders_credit_check";
ALTER TABLE "orders" ADD CONSTRAINT "orders_currency_format" CHECK ((currency)::text ~ '^[A-Z]{3}$'::text) NOT VALID;
ALTER TABLE "orders" VALIDATE CONSTRAINT "orders_currency_format";
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_provenance_check" CHECK ((((kind)::text = 'broadcast'::text) AND (broadcast_id IS NOT NULL) AND (automation_id IS NULL) AND (automation_step IS NULL) AND (automation_run_id IS NULL) AND (template_id IS NULL)) OR (((kind)::text = 'automation'::text) AND (automation_id IS NOT NULL) AND (automation_step IS NOT NULL) AND (broadcast_id IS NULL) AND (broadcast_recipient_id IS NULL) AND (template_id IS NULL)) OR (((kind)::text = 'transactional'::text) AND (broadcast_id IS NULL) AND (broadcast_recipient_id IS NULL) AND (automation_id IS NULL) AND (automation_step IS NULL) AND (automation_run_id IS NULL))) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_provenance_check";
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD CONSTRAINT "workspaces_reporting_currency_format" CHECK ((reporting_currency)::text ~ '^[A-Z]{3}$'::text) NOT VALID;
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_reporting_currency_format";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP CONSTRAINT "workspaces_reporting_currency_format";
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_provenance_check";
-- reverse: modify "orders" table
ALTER TABLE "orders" DROP CONSTRAINT "orders_currency_format", DROP CONSTRAINT "orders_credit_check";
-- reverse: modify "integrations" table
ALTER TABLE "integrations" ALTER COLUMN "channel" SET DEFAULT 'email', DROP CONSTRAINT "integrations_provider_channel_check";
-- reverse: modify "audit_entries" table
ALTER TABLE "audit_entries" DROP CONSTRAINT "audit_entries_actor_kind_check";
