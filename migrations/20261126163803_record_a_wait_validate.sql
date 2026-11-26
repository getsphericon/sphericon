-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID: a wait's cause is one of the
-- Deferral causes, and stands beside its moment (ADR 0178).
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_deferral_cause_check";
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_deferral_check";
ALTER TABLE "broadcast_recipients" VALIDATE CONSTRAINT "broadcast_recipients_deferral_cause_check";
ALTER TABLE "broadcast_recipients" VALIDATE CONSTRAINT "broadcast_recipients_deferral_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_deferral_cause_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_deferral_check";

-- +goose Down
