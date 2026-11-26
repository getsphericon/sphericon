-- +goose NO TRANSACTION
-- +goose Up
-- Validates the checks the previous migration added NOT VALID, and rebuilds the index of the Events
-- that expire by age (ADR 0019) concurrently: email.resubscribed, the return that lifts an opt-out of
-- everything (ADR 0013), never expires, so it leaves its predicate.
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_kind_check";
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_provenance_check";
ALTER TABLE "auth_attempts" VALIDATE CONSTRAINT "auth_attempts_kind_check";
ALTER TABLE "captured_messages" VALIDATE CONSTRAINT "captured_messages_surface_check";
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'sms.confirmed', 'email.complained', 'email.unsubscribed', 'email.resubscribed', 'email.engagement_tracking_granted', 'email.engagement_tracking_denied', 'sms.engagement_tracking_granted', 'sms.engagement_tracking_denied', 'telegram.engagement_tracking_granted', 'telegram.engagement_tracking_denied') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');

-- +goose Down
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'sms.confirmed', 'email.complained', 'email.unsubscribed', 'email.engagement_tracking_granted', 'email.engagement_tracking_denied', 'sms.engagement_tracking_granted', 'sms.engagement_tracking_denied', 'telegram.engagement_tracking_granted', 'telegram.engagement_tracking_denied') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');
