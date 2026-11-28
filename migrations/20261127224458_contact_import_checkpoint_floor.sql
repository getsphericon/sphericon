-- +goose Up
-- A CSV import's run counts a Contact made by a batch it runs again after a restart as created
-- (ADR 0138): the id above which a Contact of the source import was made by the batch; and how many
-- times the periodic sweep queued a run whose job was gone, never negative (validated next).
ALTER TABLE "contact_imports" ADD COLUMN "checkpoint_contact_floor" bigint NOT NULL DEFAULT 0;
ALTER TABLE "contact_imports" ADD COLUMN "recoveries" bigint NOT NULL DEFAULT 0;
ALTER TABLE "contact_imports" ADD CONSTRAINT "contact_imports_recoveries_check" CHECK (recoveries >= 0) NOT VALID;

-- +goose Down
ALTER TABLE "contact_imports" DROP CONSTRAINT "contact_imports_recoveries_check";
ALTER TABLE "contact_imports" DROP COLUMN "recoveries";
ALTER TABLE "contact_imports" DROP COLUMN "checkpoint_contact_floor";
