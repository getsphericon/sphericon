-- +goose NO TRANSACTION
-- +goose Up
-- modify "auth_attempts" table
ALTER TABLE "auth_attempts" ADD CONSTRAINT "auth_attempts_kind_check" CHECK ((kind)::text = ANY ((ARRAY['login'::character varying, 'staff_login'::character varying, 'password_reset'::character varying, 'register'::character varying, 'email_change'::character varying, 'test_send_user'::character varying, 'test_send_workspace'::character varying])::text[])) NOT VALID;
ALTER TABLE "auth_attempts" VALIDATE CONSTRAINT "auth_attempts_kind_check";
-- modify "automation_runs" table
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_failure_kind_check" CHECK ((failure_kind)::text = ANY ((ARRAY['invalid_body'::character varying, 'timeout'::character varying, 'blocked_address'::character varying, 'http_status'::character varying, 'network'::character varying])::text[])) NOT VALID, ADD CONSTRAINT "automation_runs_status_check" CHECK ((status)::text = ANY ((ARRAY['active'::character varying, 'completed'::character varying, 'failed'::character varying, 'exited'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_failure_kind_check";
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_status_check";
-- modify "automations" table
ALTER TABLE "automations" ADD CONSTRAINT "automations_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying])::text[])) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_status_check";
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'scheduled'::character varying, 'sending'::character varying, 'sent'::character varying, 'failed'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_status_check";
-- modify "confirmations" table
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID, ADD CONSTRAINT "confirmations_provenance_check" CHECK ((provenance)::text = ANY ((ARRAY['double_opt_in'::character varying, 'grandfathered'::character varying, 'imported'::character varying])::text[])) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_channel_check";
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_provenance_check";
-- modify "custom_fields" table
ALTER TABLE "custom_fields" ADD CONSTRAINT "custom_fields_type_check" CHECK ((type)::text = ANY ((ARRAY['string'::character varying, 'number'::character varying, 'bool'::character varying, 'datetime'::character varying])::text[])) NOT VALID;
ALTER TABLE "custom_fields" VALIDATE CONSTRAINT "custom_fields_type_check";
-- modify "instance_state" table
UPDATE "instance_state" SET "telemetry_enabled" = true WHERE "telemetry_enabled" IS NULL;
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "instance_state" ALTER COLUMN "telemetry_enabled" SET NOT NULL, ALTER COLUMN "telemetry_enabled" SET DEFAULT true;
-- modify "integrations" table
ALTER TABLE "integrations" ADD CONSTRAINT "integrations_channel_check" CHECK ((channel)::text = ANY ((ARRAY['email'::character varying, 'sms'::character varying])::text[])) NOT VALID, ADD CONSTRAINT "integrations_provider_check" CHECK ((provider)::text = ANY ((ARRAY['smtp'::character varying, 'ses'::character varying])::text[])) NOT VALID;
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_channel_check";
ALTER TABLE "integrations" VALIDATE CONSTRAINT "integrations_provider_check";
-- modify "invitations" table
ALTER TABLE "invitations" ADD CONSTRAINT "invitations_role_check" CHECK ((role)::text = ANY ((ARRAY['admin'::character varying, 'member'::character varying])::text[])) NOT VALID;
ALTER TABLE "invitations" VALIDATE CONSTRAINT "invitations_role_check";
-- modify "memberships" table
ALTER TABLE "memberships" ADD CONSTRAINT "memberships_role_check" CHECK ((role)::text = ANY ((ARRAY['owner'::character varying, 'admin'::character varying, 'member'::character varying])::text[])) NOT VALID;
ALTER TABLE "memberships" VALIDATE CONSTRAINT "memberships_role_check";
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID, ADD CONSTRAINT "outbound_messages_kind_check" CHECK ((kind)::text = ANY ((ARRAY['broadcast'::character varying, 'automation'::character varying, 'transactional'::character varying])::text[])) NOT VALID, ADD CONSTRAINT "outbound_messages_status_check" CHECK ((status)::text = ANY ((ARRAY['pending'::character varying, 'sent'::character varying, 'skipped'::character varying, 'failed'::character varying])::text[])) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_channel_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_kind_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_status_check";
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD CONSTRAINT "sending_domains_tracking_reason_check" CHECK ((tracking_reason)::text = 'host_taken'::text) NOT VALID, ADD CONSTRAINT "sending_domains_tracking_state_check" CHECK ((tracking_state)::text = ANY ((ARRAY['pending'::character varying, 'dns_verified'::character varying, 'serving'::character varying])::text[])) NOT VALID;
ALTER TABLE "sending_domains" VALIDATE CONSTRAINT "sending_domains_tracking_reason_check";
ALTER TABLE "sending_domains" VALIDATE CONSTRAINT "sending_domains_tracking_state_check";
-- modify "suppressions" table
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID, ADD CONSTRAINT "suppressions_reason_check" CHECK ((reason)::text = ANY ((ARRAY['bounce'::character varying, 'complaint'::character varying, 'manual'::character varying])::text[])) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_channel_check";
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_reason_check";
-- modify "tracking_host_changes" table
ALTER TABLE "tracking_host_changes" ADD CONSTRAINT "tracking_host_changes_state_check" CHECK ((state)::text = ANY ((ARRAY['dns_verified'::character varying, 'serving'::character varying, 'removed'::character varying])::text[])) NOT VALID;
ALTER TABLE "tracking_host_changes" VALIDATE CONSTRAINT "tracking_host_changes_state_check";
-- modify "unsubscribes" table
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_channel_check" CHECK ((channel)::text = 'email'::text) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_channel_check";
-- modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" ADD CONSTRAINT "webhook_endpoints_disabled_reason_check" CHECK ((disabled_reason)::text = ANY ((ARRAY['gone'::character varying, 'persistent_failure'::character varying])::text[])) NOT VALID, ADD CONSTRAINT "webhook_endpoints_last_error_kind_check" CHECK ((last_error_kind)::text = ANY ((ARRAY['timeout'::character varying, 'blocked_address'::character varying, 'http_status'::character varying, 'network'::character varying])::text[])) NOT VALID;
ALTER TABLE "webhook_endpoints" VALIDATE CONSTRAINT "webhook_endpoints_disabled_reason_check";
ALTER TABLE "webhook_endpoints" VALIDATE CONSTRAINT "webhook_endpoints_last_error_kind_check";
-- modify "workspaces" table
ALTER TABLE "workspaces" ADD CONSTRAINT "workspaces_public_id_max_length_check" CHECK (char_length((public_id)::text) <= 8) NOT VALID, ADD CONSTRAINT "workspaces_public_id_min_length_check" CHECK (char_length((public_id)::text) >= 8) NOT VALID, ADD CONSTRAINT "workspaces_suspended_by_kind_check" CHECK ((suspended_by_kind)::text = ANY ((ARRAY['system'::character varying, 'cli'::character varying, 'staff'::character varying])::text[])) NOT VALID;
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_public_id_max_length_check";
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_public_id_min_length_check";
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_suspended_by_kind_check";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" DROP CONSTRAINT "workspaces_suspended_by_kind_check", DROP CONSTRAINT "workspaces_public_id_min_length_check", DROP CONSTRAINT "workspaces_public_id_max_length_check";
-- reverse: modify "webhook_endpoints" table
ALTER TABLE "webhook_endpoints" DROP CONSTRAINT "webhook_endpoints_last_error_kind_check", DROP CONSTRAINT "webhook_endpoints_disabled_reason_check";
-- reverse: modify "unsubscribes" table
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_channel_check";
-- reverse: modify "tracking_host_changes" table
ALTER TABLE "tracking_host_changes" DROP CONSTRAINT "tracking_host_changes_state_check";
-- reverse: modify "suppressions" table
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_reason_check", DROP CONSTRAINT "suppressions_channel_check";
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP CONSTRAINT "sending_domains_tracking_state_check", DROP CONSTRAINT "sending_domains_tracking_reason_check";
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_status_check", DROP CONSTRAINT "outbound_messages_kind_check", DROP CONSTRAINT "outbound_messages_channel_check";
-- reverse: modify "memberships" table
ALTER TABLE "memberships" DROP CONSTRAINT "memberships_role_check";
-- reverse: modify "invitations" table
ALTER TABLE "invitations" DROP CONSTRAINT "invitations_role_check";
-- reverse: modify "integrations" table
ALTER TABLE "integrations" DROP CONSTRAINT "integrations_provider_check", DROP CONSTRAINT "integrations_channel_check";
-- reverse: modify "instance_state" table
ALTER TABLE "instance_state" ALTER COLUMN "telemetry_enabled" DROP NOT NULL, ALTER COLUMN "telemetry_enabled" DROP DEFAULT;
-- reverse: modify "custom_fields" table
ALTER TABLE "custom_fields" DROP CONSTRAINT "custom_fields_type_check";
-- reverse: modify "confirmations" table
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_provenance_check", DROP CONSTRAINT "confirmations_channel_check";
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_status_check";
-- reverse: modify "automations" table
ALTER TABLE "automations" DROP CONSTRAINT "automations_status_check";
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_status_check", DROP CONSTRAINT "automation_runs_failure_kind_check";
-- reverse: modify "auth_attempts" table
ALTER TABLE "auth_attempts" DROP CONSTRAINT "auth_attempts_kind_check";
