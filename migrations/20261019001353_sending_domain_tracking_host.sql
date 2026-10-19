-- +goose NO TRANSACTION
-- +goose Up
-- create index "sendingdomain_domain" to table: "sending_domains"
CREATE UNIQUE INDEX CONCURRENTLY "sendingdomain_domain" ON "sending_domains" ("domain") WHERE (tracking_state IN ('dns_verified', 'serving'));

-- +goose Down
-- reverse: create index "sendingdomain_domain" to table: "sending_domains"
DROP INDEX CONCURRENTLY "sendingdomain_domain";
