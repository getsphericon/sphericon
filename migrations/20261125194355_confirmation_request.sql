-- +goose Up
-- The confirmation email (ADR 0013): an Outbound message of the kind confirmation, which points at
-- no surface record and is captured as its own surface, and the throttle that counts the confirmation emails of a Destination. The
-- checks are written by hand (ent's diff leaves a CHECK as it is): each is added NOT VALID and the
-- next migration (20261125194356) validates it and rebuilds concurrently the retention index whose
-- predicate now leaves out email.resubscribed (ADR 0019).
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_kind_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_kind_check" CHECK (kind IN ('broadcast', 'automation', 'transactional', 'confirmation')) NOT VALID;
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_provenance_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_provenance_check" CHECK ((kind = 'broadcast' AND broadcast_id IS NOT NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL AND template_id IS NULL) OR (kind = 'automation' AND automation_id IS NOT NULL AND automation_version_id IS NOT NULL AND automation_step_id IS NOT NULL AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND template_id IS NULL) OR (kind = 'transactional' AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL) OR (kind = 'confirmation' AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL AND template_id IS NULL)) NOT VALID;
ALTER TABLE "auth_attempts" DROP CONSTRAINT "auth_attempts_kind_check";
ALTER TABLE "auth_attempts" ADD CONSTRAINT "auth_attempts_kind_check" CHECK (kind IN ('login', 'staff_login', 'password_reset', 'sign_up', 'email_change', 'test_send_user', 'test_send_workspace', 'confirmation')) NOT VALID;
ALTER TABLE "captured_messages" DROP CONSTRAINT "captured_messages_surface_check";
ALTER TABLE "captured_messages" ADD CONSTRAINT "captured_messages_surface_check" CHECK (surface IN ('broadcast', 'automation', 'transactional', 'confirmation', 'test')) NOT VALID;

-- +goose Down
ALTER TABLE "captured_messages" DROP CONSTRAINT "captured_messages_surface_check";
ALTER TABLE "captured_messages" ADD CONSTRAINT "captured_messages_surface_check" CHECK (surface IN ('broadcast', 'automation', 'transactional', 'test'));
ALTER TABLE "auth_attempts" DROP CONSTRAINT "auth_attempts_kind_check";
ALTER TABLE "auth_attempts" ADD CONSTRAINT "auth_attempts_kind_check" CHECK (kind IN ('login', 'staff_login', 'password_reset', 'sign_up', 'email_change', 'test_send_user', 'test_send_workspace'));
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_provenance_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_provenance_check" CHECK ((kind = 'broadcast' AND broadcast_id IS NOT NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL AND template_id IS NULL) OR (kind = 'automation' AND automation_id IS NOT NULL AND automation_version_id IS NOT NULL AND automation_step_id IS NOT NULL AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND template_id IS NULL) OR (kind = 'transactional' AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL));
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_kind_check";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_kind_check" CHECK (kind IN ('broadcast', 'automation', 'transactional'));
