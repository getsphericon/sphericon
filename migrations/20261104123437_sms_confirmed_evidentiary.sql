-- +goose NO TRANSACTION
-- +goose Up
-- An SMS Confirmation's proof is the sms.confirmed Event (marketing.confirmed is email's), and
-- consent proof is evidentiary (ADR 0019): the retention index leaves it out, so it is rebuilt with
-- the new action. ent's diff does not see a changed partial-index predicate, so this migration is
-- written by hand. No stored row is rewritten: no Instance holds SMS consent Events yet.
-- drop index "events_created_at_analytical_idx" from table: "events"
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
-- create index "events_created_at_analytical_idx" to table: "events"
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'sms.confirmed', 'email.complained', 'email.unsubscribed') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');

-- +goose Down
-- reverse: create index "events_created_at_analytical_idx" to table: "events"
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
-- reverse: drop index "events_created_at_analytical_idx" from table: "events"
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'email.complained', 'email.unsubscribed') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');
