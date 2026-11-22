-- +goose Up
-- modify "rule_versions" table
ALTER TABLE "rule_versions" ALTER COLUMN "condition" SET DEFAULT '{"kind": "group", "entries": [], "combinator": "and"}';
-- modify "segments" table
ALTER TABLE "segments" ALTER COLUMN "condition" SET DEFAULT '{"kind": "group", "entries": [], "combinator": "and"}';

-- +goose Down
-- reverse: modify "segments" table
ALTER TABLE "segments" ALTER COLUMN "condition" SET DEFAULT '{"kind": "group", "rules": [], "combinator": "and"}';
-- reverse: modify "rule_versions" table
ALTER TABLE "rule_versions" ALTER COLUMN "condition" SET DEFAULT '{"kind": "group", "rules": [], "combinator": "and"}';
