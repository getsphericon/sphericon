-- +goose Up
-- A Contact a CSV import creates has the source `import` (ADR 0138). ent's diff does not see a
-- changed enum check, so the sources' check is replaced by hand: added NOT VALID, and the next
-- migration (20261128003002) validates it.
ALTER TABLE "contacts" DROP CONSTRAINT "contacts_source_check";
ALTER TABLE "contacts" ADD CONSTRAINT "contacts_source_check" CHECK ((source)::text = ANY ((ARRAY['manual'::character varying, 'api'::character varying, 'tracker'::character varying, 'import'::character varying])::text[])) NOT VALID;

-- +goose Down
ALTER TABLE "contacts" DROP CONSTRAINT "contacts_source_check";
ALTER TABLE "contacts" ADD CONSTRAINT "contacts_source_check" CHECK ((source)::text = ANY ((ARRAY['manual'::character varying, 'api'::character varying, 'tracker'::character varying])::text[]));
