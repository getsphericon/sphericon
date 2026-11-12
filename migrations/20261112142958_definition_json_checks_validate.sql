-- +goose NO TRANSACTION
-- +goose Up
-- An empty definition was read as none by the decoders, as '[]' is: it becomes '[]'. The one
-- Condition in a format the parser dropped is the seeded fixture Segment's, which the API (it
-- encodes a group) never wrote: it becomes what the fixture holds now. Any other row that fails is a
-- document the app could not decode already; it is left for a person, never rewritten (an emptied
-- Condition would match every Contact).
UPDATE "automation_versions" SET "steps" = '[]' WHERE "steps" = '';
UPDATE "rule_versions" SET "actions" = '[]' WHERE "actions" = '';
UPDATE "segments" SET "condition" = '{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"notNull"}]}' WHERE "condition" = 'status == ''active''';
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_steps_check";
ALTER TABLE "broadcasts" VALIDATE CONSTRAINT "broadcasts_condition_check";
ALTER TABLE "rule_versions" VALIDATE CONSTRAINT "rule_versions_actions_check";
ALTER TABLE "rule_versions" VALIDATE CONSTRAINT "rule_versions_condition_check";
ALTER TABLE "segments" VALIDATE CONSTRAINT "segments_condition_check";

-- +goose Down
SELECT 1;
