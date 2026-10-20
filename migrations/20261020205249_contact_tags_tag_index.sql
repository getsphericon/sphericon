-- +goose NO TRANSACTION
-- +goose Up
-- create index "contacttag_tag_id" to table: "contact_tags"
CREATE INDEX CONCURRENTLY "contacttag_tag_id" ON "contact_tags" ("tag_id");

-- +goose Down
-- reverse: create index "contacttag_tag_id" to table: "contact_tags"
DROP INDEX CONCURRENTLY "contacttag_tag_id";
