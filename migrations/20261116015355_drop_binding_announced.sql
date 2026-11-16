-- +goose Up
-- Destructive (ADR 0033, ADR 0171): a Visitor's Binding reaches no Contact, so a Binding is announced
-- when a Contact gets it and the flag that tracked a Visitor's announcement goes.
-- modify "bindings" table
ALTER TABLE "bindings" DROP COLUMN "announced";

-- +goose Down
-- reverse: modify "bindings" table
ALTER TABLE "bindings" ADD COLUMN "announced" boolean NOT NULL DEFAULT false;
