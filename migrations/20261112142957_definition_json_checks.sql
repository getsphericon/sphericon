-- +goose Up
-- The definition documents (an Automation's steps, a Rule's Condition and Actions, a Segment's and
-- an armed Broadcast's Condition) are JSON the app decodes: the database refuses text that is not.
-- Added NOT VALID under the runner's bounded lock wait, validated by the next migration.
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_steps_check" CHECK ((steps)::text IS JSON ARRAY) NOT VALID;
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_condition_check" CHECK (((condition)::text = ''::text) OR ((condition)::text IS JSON OBJECT)) NOT VALID;
ALTER TABLE "rule_versions" ADD CONSTRAINT "rule_versions_actions_check" CHECK ((actions)::text IS JSON ARRAY) NOT VALID;
ALTER TABLE "rule_versions" ADD CONSTRAINT "rule_versions_condition_check" CHECK (((condition)::text = ''::text) OR ((condition)::text IS JSON OBJECT)) NOT VALID;
ALTER TABLE "segments" ADD CONSTRAINT "segments_condition_check" CHECK (((condition)::text = ''::text) OR ((condition)::text IS JSON OBJECT)) NOT VALID;

-- +goose Down
ALTER TABLE "segments" DROP CONSTRAINT "segments_condition_check";
ALTER TABLE "rule_versions" DROP CONSTRAINT "rule_versions_condition_check", DROP CONSTRAINT "rule_versions_actions_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_condition_check";
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_steps_check";
