-- +goose Up
-- A CSV import's run counts a Contact made by a batch it runs again after a restart as created
-- (ADR 0138): the id above which a Contact of the source import was made by the batch.
ALTER TABLE "contact_imports" ADD COLUMN "checkpoint_contact_floor" bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE "contact_imports" DROP COLUMN "checkpoint_contact_floor";
