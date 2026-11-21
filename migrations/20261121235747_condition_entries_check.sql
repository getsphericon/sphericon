-- +goose Up
-- A Condition's group holds its entries as a list (ADR 0181): a tree without them, as a release
-- before the typed tree writes it under `rules`, would read as the empty group, which matches every
-- Contact, so the database refuses it. It sorts after the Go migration that rewrites every stored
-- Condition (20261121205037). Each CHECK is added NOT VALID under the runner's bounded lock wait,
-- and validated by the next migration.
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD CONSTRAINT "broadcasts_condition_entries_check" CHECK ((condition IS NULL) OR COALESCE((jsonb_typeof((condition -> 'entries'::text)) = 'array'::text), false)) NOT VALID;
-- modify "rule_versions" table
ALTER TABLE "rule_versions" ADD CONSTRAINT "rule_versions_condition_entries_check" CHECK ((condition IS NULL) OR COALESCE((jsonb_typeof((condition -> 'entries'::text)) = 'array'::text), false)) NOT VALID;
-- modify "segments" table
ALTER TABLE "segments" ADD CONSTRAINT "segments_condition_entries_check" CHECK ((condition IS NULL) OR COALESCE((jsonb_typeof((condition -> 'entries'::text)) = 'array'::text), false)) NOT VALID;

-- +goose Down
-- reverse: modify "segments" table
ALTER TABLE "segments" DROP CONSTRAINT "segments_condition_entries_check";
-- reverse: modify "rule_versions" table
ALTER TABLE "rule_versions" DROP CONSTRAINT "rule_versions_condition_entries_check";
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP CONSTRAINT "broadcasts_condition_entries_check";
