-- +goose Up
-- A wait is recorded where it waits (ADR 0178): an Enrollment keeps since when and why the send of
-- its Step waits, a Broadcast recipient why it waits beside until when, and an Outbound message
-- since when and why it waited. ent's diff wrote the columns and the checks; every check is added
-- NOT VALID, and the next migration (20261126163803) validates them.
ALTER TABLE "automation_runs" ADD COLUMN "deferred_since" timestamptz NULL, ADD COLUMN "deferral_cause" character varying NULL;
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_deferral_cause_check" CHECK ((deferral_cause)::text = ANY ((ARRAY['provider_limit'::character varying, 'send_cap'::character varying, 'provider'::character varying, 'sending_window'::character varying, 'frequency_cap'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_deferral_check" CHECK ((deferred_since IS NULL) = (deferral_cause IS NULL)) NOT VALID;
ALTER TABLE "broadcast_recipients" ADD COLUMN "deferral_cause" character varying NULL;
ALTER TABLE "broadcast_recipients" ADD CONSTRAINT "broadcast_recipients_deferral_cause_check" CHECK ((deferral_cause)::text = ANY ((ARRAY['provider_limit'::character varying, 'send_cap'::character varying, 'provider'::character varying, 'sending_window'::character varying, 'frequency_cap'::character varying])::text[])) NOT VALID;
ALTER TABLE "broadcast_recipients" ADD CONSTRAINT "broadcast_recipients_deferral_check" CHECK ((deferral_cause IS NULL) OR (deferred_until IS NOT NULL)) NOT VALID;
ALTER TABLE "outbound_messages" ADD COLUMN "deferred_since" timestamptz NULL, ADD COLUMN "deferral_cause" character varying NULL;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_deferral_cause_check" CHECK ((deferral_cause)::text = ANY ((ARRAY['provider_limit'::character varying, 'send_cap'::character varying, 'provider'::character varying, 'sending_window'::character varying, 'frequency_cap'::character varying])::text[])) NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_deferral_check" CHECK ((deferred_since IS NULL) = (deferral_cause IS NULL)) NOT VALID;

-- +goose Down
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_deferral_check", DROP CONSTRAINT "outbound_messages_deferral_cause_check", DROP COLUMN "deferral_cause", DROP COLUMN "deferred_since";
ALTER TABLE "broadcast_recipients" DROP CONSTRAINT "broadcast_recipients_deferral_check", DROP CONSTRAINT "broadcast_recipients_deferral_cause_check", DROP COLUMN "deferral_cause";
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_deferral_check", DROP CONSTRAINT "automation_runs_deferral_cause_check", DROP COLUMN "deferral_cause", DROP COLUMN "deferred_since";
