-- +goose NO TRANSACTION
-- +goose Up
-- The Workspace's Engagement tracking default and the two facts of an Outbound message's tracking
-- (ADR 0120): whether it went out tracked (true until a render decides, as every message before
-- this went out) and when its tracking was withdrawn. ent's diff wrote them; the default's check is
-- added NOT VALID and validated on its own, each statement in a transaction of its own.
ALTER TABLE "outbound_messages" ADD COLUMN "tracked" boolean NOT NULL DEFAULT true, ADD COLUMN "tracking_withdrawn_at" timestamptz NULL;
ALTER TABLE "workspaces" ADD COLUMN "engagement_tracking_default" character varying NOT NULL DEFAULT 'track';
ALTER TABLE "workspaces" ADD CONSTRAINT "workspaces_engagement_tracking_default_check" CHECK ((engagement_tracking_default)::text = ANY ((ARRAY['track'::character varying, 'dont_track'::character varying])::text[])) NOT VALID;
ALTER TABLE "workspaces" VALIDATE CONSTRAINT "workspaces_engagement_tracking_default_check";

-- +goose Down
ALTER TABLE "workspaces" DROP CONSTRAINT "workspaces_engagement_tracking_default_check";
ALTER TABLE "workspaces" DROP COLUMN "engagement_tracking_default";
ALTER TABLE "outbound_messages" DROP COLUMN "tracking_withdrawn_at", DROP COLUMN "tracked";
