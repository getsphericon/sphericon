-- +goose NO TRANSACTION
-- +goose Up
-- create index "broadcast_integration_id" to table: "broadcasts"
CREATE INDEX CONCURRENTLY "broadcast_integration_id" ON "broadcasts" ("integration_id");
-- create index "broadcast_segment_id" to table: "broadcasts"
CREATE INDEX CONCURRENTLY "broadcast_segment_id" ON "broadcasts" ("segment_id");
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_integrations_integration";
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_segments_segment" FOREIGN KEY ("segment_id") REFERENCES "segments" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_segments_segment";

-- +goose Down
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_segments_segment";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_integrations_integration";
-- reverse: create index "broadcast_segment_id" to table: "broadcasts"
DROP INDEX CONCURRENTLY "broadcast_segment_id";
-- reverse: create index "broadcast_integration_id" to table: "broadcasts"
DROP INDEX CONCURRENTLY "broadcast_integration_id";
