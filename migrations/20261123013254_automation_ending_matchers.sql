-- +goose Up
-- A version's exit and goal events are Event matchers (ADR 0133): an action and an optional filter
-- on the Event's own properties, the exits a jsonb list and the goal a jsonb object, NULL for none
-- (ADR 0169). ent's diff wrote a drop and an add; by hand, the new columns come first, every
-- stored exit and goal event becomes a matcher without a filter, a guard refuses to drop the old
-- columns unless each was carried over, and the constraints of the new columns are added NOT VALID:
-- the next migration validates them.
ALTER TABLE "automation_versions" ADD COLUMN "exits" jsonb NOT NULL DEFAULT '[]', ADD COLUMN "goal" jsonb NULL;
UPDATE "automation_versions" SET
  "exits" = (SELECT COALESCE(jsonb_agg(jsonb_build_object('action', e.action) ORDER BY e.n), '[]'::jsonb)
             FROM jsonb_array_elements_text("exit_events") WITH ORDINALITY AS e(action, n)),
  "goal" = CASE WHEN "goal_event" IS NULL THEN NULL ELSE jsonb_build_object('action', "goal_event") END
WHERE "exit_events" <> '[]'::jsonb OR "goal_event" IS NOT NULL;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "automation_versions"
    WHERE jsonb_array_length("exits") <> jsonb_array_length("exit_events")
      OR ("goal" IS NULL) <> ("goal_event" IS NULL)) THEN
    RAISE EXCEPTION 'an exit or goal event of an Automation version was not carried over to its matchers (ADR 0133)';
  END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE "automation_versions" DROP COLUMN "exit_events", DROP COLUMN "goal_event";
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_exits_check" CHECK (jsonb_typeof(exits) = 'array'::text) NOT VALID;
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_goal_check" CHECK (jsonb_typeof(goal) = 'object'::text) NOT VALID;

-- +goose Down
-- The matchers go back to their actions; a filter is lost.
ALTER TABLE "automation_versions" ADD COLUMN "goal_event" character varying NULL, ADD COLUMN "exit_events" jsonb NOT NULL DEFAULT '[]';
UPDATE "automation_versions" SET
  "exit_events" = (SELECT COALESCE(jsonb_agg(e.m -> 'action' ORDER BY e.n), '[]'::jsonb)
                   FROM jsonb_array_elements("exits") WITH ORDINALITY AS e(m, n)),
  "goal_event" = "goal" ->> 'action'
WHERE "exits" <> '[]'::jsonb OR "goal" IS NOT NULL;
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_goal_check", DROP CONSTRAINT "automation_versions_exits_check", DROP COLUMN "goal", DROP COLUMN "exits";
