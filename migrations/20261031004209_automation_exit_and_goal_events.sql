-- +goose NO TRANSACTION
-- +goose Up
-- Exit and goal events of an Automation version (#751), when an Enrollment began (the triggering
-- Event's occurred_at), and `goal_reached`, a new end of an Enrollment. Every version written
-- before has no exit and no goal event. ent's diff does not see a changed enum CHECK, so the
-- status constraint is swapped by hand: the new one is added NOT VALID and validated after.
-- modify "automation_runs" table
ALTER TABLE "automation_runs" ADD COLUMN "began_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_status_check";
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_status_check" CHECK ((status)::text = ANY ((ARRAY['active'::character varying, 'completed'::character varying, 'failed'::character varying, 'exited'::character varying, 'goal_reached'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_status_check";
-- modify "automation_versions" table
ALTER TABLE "automation_versions" ADD COLUMN "exit_events" jsonb NOT NULL DEFAULT '[]', ADD COLUMN "goal_event" character varying NULL;

-- +goose Down
-- reverse: modify "automation_versions" table
ALTER TABLE "automation_versions" DROP COLUMN "goal_event", DROP COLUMN "exit_events";
-- An Enrollment that reached its goal left early: it goes back as exited.
ALTER TABLE "automation_runs" DROP CONSTRAINT "automation_runs_status_check";
UPDATE "automation_runs" SET "status" = 'exited' WHERE "status" = 'goal_reached';
ALTER TABLE "automation_runs" ADD CONSTRAINT "automation_runs_status_check" CHECK ((status)::text = ANY ((ARRAY['active'::character varying, 'completed'::character varying, 'failed'::character varying, 'exited'::character varying])::text[])) NOT VALID;
ALTER TABLE "automation_runs" VALIDATE CONSTRAINT "automation_runs_status_check";
-- reverse: modify "automation_runs" table
ALTER TABLE "automation_runs" DROP COLUMN "began_at";
