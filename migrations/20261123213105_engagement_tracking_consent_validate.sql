-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID, and rebuilds the index of the Events
-- that expire by age (ADR 0019) concurrently: the evidentiary Events of Engagement tracking consent
-- (ADR 0120) never expire, so they leave its predicate.
ALTER TABLE "consent_stamps" VALIDATE CONSTRAINT "consent_stamps_engagement_tracking_check";
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'sms.confirmed', 'email.complained', 'email.unsubscribed', 'email.engagement_tracking_granted', 'email.engagement_tracking_denied', 'sms.engagement_tracking_granted', 'sms.engagement_tracking_denied', 'telegram.engagement_tracking_granted', 'telegram.engagement_tracking_denied') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');

-- +goose Down
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'sms.confirmed', 'email.complained', 'email.unsubscribed') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');
