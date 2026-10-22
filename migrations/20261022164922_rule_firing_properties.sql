-- +goose Up
-- modify "rule_firings" table
ALTER TABLE "rule_firings" ADD COLUMN "properties" character varying NULL;

-- +goose Down
-- reverse: modify "rule_firings" table
ALTER TABLE "rule_firings" DROP COLUMN "properties";
