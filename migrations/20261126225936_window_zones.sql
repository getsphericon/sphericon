-- +goose Up
-- How a Sending window judged an Outbound message (ADR 0134), and the figures that count the sends a
-- single zone judged. ent's diff wrote the columns and the check; the check is added NOT VALID, and
-- the next migration (20261126225937) validates it.
ALTER TABLE "automation_figures" ADD COLUMN "single_zone" bigint NOT NULL DEFAULT 0;
ALTER TABLE "broadcast_figures" ADD COLUMN "single_zone" bigint NOT NULL DEFAULT 0;
ALTER TABLE "outbound_messages" ADD COLUMN "window_zones" character varying NOT NULL DEFAULT 'unjudged';
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_window_zones_check" CHECK ((window_zones)::text = ANY ((ARRAY['unjudged'::character varying, 'no_zone'::character varying, 'one_zone'::character varying, 'several_zones'::character varying])::text[])) NOT VALID;

-- +goose Down
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_window_zones_check", DROP COLUMN "window_zones";
ALTER TABLE "broadcast_figures" DROP COLUMN "single_zone";
ALTER TABLE "automation_figures" DROP COLUMN "single_zone";
