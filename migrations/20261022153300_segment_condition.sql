-- +goose Up
-- Breaking rename (ADR 0033, ADR 0054): a Segment's stored tree is a Condition, shared with the Rule.
-- The column keeps its type and its values; ent's diff would drop and add, losing the trees.
-- modify "segments" table
ALTER TABLE "segments" RENAME COLUMN "definition" TO "condition";

-- +goose Down
-- reverse: modify "segments" table
ALTER TABLE "segments" RENAME COLUMN "condition" TO "definition";
