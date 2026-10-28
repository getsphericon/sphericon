-- +goose NO TRANSACTION
-- +goose Up
-- A bounce Event's bounceKind is "hard" or "soft" (it was "permanent" or "transient"). The
-- retention index excludes hard bounces, so it is rebuilt with the new literal. ent's diff does
-- not see a changed partial-index predicate, so this migration is written by hand. No stored
-- value is rewritten: no Instance holds bounce Events yet.
-- drop index "events_created_at_analytical_idx" from table: "events"
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
-- create index "events_created_at_analytical_idx" to table: "events"
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'email.complained', 'email.unsubscribed') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'hard');

-- +goose Down
-- reverse: create index "events_created_at_analytical_idx" to table: "events"
DROP INDEX CONCURRENTLY "events_created_at_analytical_idx";
-- reverse: drop index "events_created_at_analytical_idx" from table: "events"
CREATE INDEX CONCURRENTLY "events_created_at_analytical_idx" ON "events" ("created_at") WHERE action NOT IN ('marketing.confirmed', 'email.complained', 'email.unsubscribed') AND NOT (action = 'email.bounced' AND COALESCE(properties->>'bounceKind', '') = 'permanent');
