-- +goose Up
-- modify "brand_kits" table
ALTER TABLE "brand_kits" ADD COLUMN "website_url" character varying NOT NULL DEFAULT '';

-- +goose Down
-- reverse: modify "brand_kits" table
ALTER TABLE "brand_kits" DROP COLUMN "website_url";
