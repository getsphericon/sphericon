-- +goose NO TRANSACTION
-- +goose Up
-- create index "rulefiring_created_at" to table: "rule_firings"
CREATE INDEX CONCURRENTLY "rulefiring_created_at" ON "rule_firings" ("created_at");

-- +goose Down
-- reverse: create index "rulefiring_created_at" to table: "rule_firings"
DROP INDEX CONCURRENTLY "rulefiring_created_at";
