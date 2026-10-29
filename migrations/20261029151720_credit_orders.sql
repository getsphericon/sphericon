-- +goose Up
-- modify "orders" table
ALTER TABLE "orders" ADD COLUMN "credited_click_at" timestamptz NULL, ADD COLUMN "broadcast_id" bigint NULL, ADD COLUMN "automation_id" bigint NULL, ADD CONSTRAINT "orders_automations_automation" FOREIGN KEY ("automation_id") REFERENCES "automations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT, ADD CONSTRAINT "orders_broadcasts_broadcast" FOREIGN KEY ("broadcast_id") REFERENCES "broadcasts" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT;
-- create index "order_automation_id" to table: "orders"
CREATE INDEX "order_automation_id" ON "orders" ("automation_id");
-- create index "order_broadcast_id" to table: "orders"
CREATE INDEX "order_broadcast_id" ON "orders" ("broadcast_id");

-- +goose Down
-- reverse: create index "order_broadcast_id" to table: "orders"
DROP INDEX "order_broadcast_id";
-- reverse: create index "order_automation_id" to table: "orders"
DROP INDEX "order_automation_id";
-- reverse: modify "orders" table
ALTER TABLE "orders" DROP CONSTRAINT "orders_broadcasts_broadcast", DROP CONSTRAINT "orders_automations_automation", DROP COLUMN "automation_id", DROP COLUMN "broadcast_id", DROP COLUMN "credited_click_at";
