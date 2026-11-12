-- +goose Up
-- modify "brand_kits" table
ALTER TABLE "brand_kits" ADD COLUMN "revision" bigint NOT NULL DEFAULT 1;
-- Revision 1 is a Brand kit not saved yet (ADR 0119): a saved one is at least 2, the revision of
-- its first save.
UPDATE "brand_kits" SET "revision" = 2;

-- +goose Down
-- reverse: modify "brand_kits" table
ALTER TABLE "brand_kits" DROP COLUMN "revision";
