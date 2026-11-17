-- +goose Up
-- ADR 0169: the definition documents (an Automation version's steps, a Rule version's Condition
-- and Actions, a Segment's and an armed Broadcast's Condition) are jsonb holding their Go types.
-- The text checks go first, then '' and a missing Condition of a Segment or a Rule version become
-- the empty group, which matches every Contact as they did, and a check of the JSON type replaces
-- each text check. The type change rewrites each table under its lock whatever follows, so the
-- NOT NULL and the checks, made in the same transaction, add a scan of the rewritten rows and no
-- lock of their own: adding them NOT VALID would save nothing.
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_steps_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_condition_check";
ALTER TABLE "rule_versions" DROP CONSTRAINT "rule_versions_actions_check", DROP CONSTRAINT "rule_versions_condition_check";
ALTER TABLE "segments" DROP CONSTRAINT "segments_condition_check";

UPDATE "broadcasts" SET "condition" = '{"kind":"group","combinator":"and","rules":[]}' WHERE "condition" = '';
UPDATE "rule_versions" SET "condition" = '{"kind":"group","combinator":"and","rules":[]}' WHERE "condition" IS NULL OR "condition" = '';
UPDATE "segments" SET "condition" = '{"kind":"group","combinator":"and","rules":[]}' WHERE "condition" IS NULL OR "condition" = '';

ALTER TABLE "automation_versions" ALTER COLUMN "steps" DROP DEFAULT, ALTER COLUMN "steps" TYPE jsonb USING "steps"::jsonb, ALTER COLUMN "steps" SET DEFAULT '[]';
ALTER TABLE "broadcasts" ALTER COLUMN "condition" TYPE jsonb USING "condition"::jsonb;
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "rule_versions" ALTER COLUMN "condition" TYPE jsonb USING "condition"::jsonb, ALTER COLUMN "condition" SET NOT NULL, ALTER COLUMN "condition" SET DEFAULT '{"kind": "group", "rules": [], "combinator": "and"}', ALTER COLUMN "actions" DROP DEFAULT, ALTER COLUMN "actions" TYPE jsonb USING "actions"::jsonb, ALTER COLUMN "actions" SET DEFAULT '[]';
-- squawk-ignore adding-not-nullable-field
ALTER TABLE "segments" ALTER COLUMN "condition" TYPE jsonb USING "condition"::jsonb, ALTER COLUMN "condition" SET NOT NULL, ALTER COLUMN "condition" SET DEFAULT '{"kind": "group", "rules": [], "combinator": "and"}';

-- squawk-ignore constraint-missing-not-valid
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_steps_check" CHECK (jsonb_typeof(steps) = 'array');
-- squawk-ignore constraint-missing-not-valid
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_condition_check" CHECK (jsonb_typeof(condition) = 'object');
-- squawk-ignore constraint-missing-not-valid
ALTER TABLE "rule_versions" ADD CONSTRAINT "rule_versions_actions_check" CHECK (jsonb_typeof(actions) = 'array'), ADD CONSTRAINT "rule_versions_condition_check" CHECK (jsonb_typeof(condition) = 'object');
-- squawk-ignore constraint-missing-not-valid
ALTER TABLE "segments" ADD CONSTRAINT "segments_condition_check" CHECK (jsonb_typeof(condition) = 'object');

-- +goose Down
ALTER TABLE "segments" DROP CONSTRAINT "segments_condition_check";
ALTER TABLE "rule_versions" DROP CONSTRAINT "rule_versions_condition_check", DROP CONSTRAINT "rule_versions_actions_check";
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_condition_check";
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_steps_check";
ALTER TABLE "segments" ALTER COLUMN "condition" TYPE character varying, ALTER COLUMN "condition" DROP NOT NULL, ALTER COLUMN "condition" DROP DEFAULT;
ALTER TABLE "rule_versions" ALTER COLUMN "actions" DROP DEFAULT, ALTER COLUMN "actions" TYPE character varying, ALTER COLUMN "actions" SET DEFAULT '[]', ALTER COLUMN "condition" TYPE character varying, ALTER COLUMN "condition" DROP NOT NULL, ALTER COLUMN "condition" DROP DEFAULT;
ALTER TABLE "broadcasts" ALTER COLUMN "condition" TYPE character varying;
ALTER TABLE "automation_versions" ALTER COLUMN "steps" DROP DEFAULT, ALTER COLUMN "steps" TYPE character varying, ALTER COLUMN "steps" SET DEFAULT '[]';
ALTER TABLE "segments" ADD CONSTRAINT "segments_condition_check" CHECK (((condition)::text = ''::text) OR ((condition)::text IS JSON OBJECT));
ALTER TABLE "rule_versions" ADD CONSTRAINT "rule_versions_actions_check" CHECK ((actions)::text IS JSON ARRAY), ADD CONSTRAINT "rule_versions_condition_check" CHECK (((condition)::text = ''::text) OR ((condition)::text IS JSON OBJECT));
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_condition_check" CHECK (((condition)::text = ''::text) OR ((condition)::text IS JSON OBJECT));
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_steps_check" CHECK ((steps)::text IS JSON ARRAY);
