-- +goose NO TRANSACTION
-- +goose Up
-- A held order Event is kept as sent (its properties), not as an amount, so an Order Completed
-- that names no currency can wait for the Reporting currency (ADR 0064). ent's diff does not see a
-- changed enum CHECK, so the check is swapped by hand. The rows held so far hold an amount in a
-- currency and cannot be turned back into what was sent, so they are dropped: they are Events whose
-- Order Completed never arrived, and nothing has shipped with them.
DELETE FROM "held_order_events";
ALTER TABLE "held_order_events" DROP COLUMN "amount", DROP COLUMN "currency", DROP COLUMN "refund_id", ADD COLUMN "properties" jsonb NOT NULL, ADD COLUMN "contact_id" bigint NULL;
ALTER TABLE "held_order_events" DROP CONSTRAINT "held_order_events_kind_check";
ALTER TABLE "held_order_events" ADD CONSTRAINT "held_order_events_kind_check" CHECK ((kind)::text = ANY ((ARRAY['completed'::character varying, 'updated'::character varying, 'refunded'::character varying, 'cancelled'::character varying])::text[])) NOT VALID;
ALTER TABLE "held_order_events" VALIDATE CONSTRAINT "held_order_events_kind_check";

-- +goose Down
DELETE FROM "held_order_events";
ALTER TABLE "held_order_events" DROP CONSTRAINT "held_order_events_kind_check";
ALTER TABLE "held_order_events" ADD CONSTRAINT "held_order_events_kind_check" CHECK ((kind)::text = ANY ((ARRAY['updated'::character varying, 'refunded'::character varying, 'cancelled'::character varying])::text[])) NOT VALID;
ALTER TABLE "held_order_events" VALIDATE CONSTRAINT "held_order_events_kind_check";
ALTER TABLE "held_order_events" DROP COLUMN "contact_id", DROP COLUMN "properties", ADD COLUMN "refund_id" character varying NULL, ADD COLUMN "currency" character varying NOT NULL, ADD COLUMN "amount" bigint NULL;
