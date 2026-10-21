-- +goose NO TRANSACTION
-- +goose Up
-- create index "joindomain_domain" to table: "join_domains"
CREATE UNIQUE INDEX CONCURRENTLY "joindomain_domain" ON "join_domains" ("domain") WHERE (verified_at IS NOT NULL);

-- +goose Down
-- reverse: create index "joindomain_domain" to table: "join_domains"
DROP INDEX CONCURRENTLY "joindomain_domain";
