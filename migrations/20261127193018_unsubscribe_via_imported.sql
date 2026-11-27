-- +goose Up
-- An Unsubscribe an import brings from another platform is `via: imported` (ADR 0138). ent's diff
-- does not see a changed enum check, so the check is replaced by hand: added NOT VALID, and the
-- next migration validates it.
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_via_check";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_via_check" CHECK ((via)::text = ANY ((ARRAY['keyword'::character varying, 'button'::character varying, 'block'::character varying, 'link'::character varying, 'refused_send'::character varying, 'manual'::character varying, 'imported'::character varying])::text[])) NOT VALID;

-- +goose Down
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_via_check";
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_via_check" CHECK ((via)::text = ANY ((ARRAY['keyword'::character varying, 'button'::character varying, 'block'::character varying, 'link'::character varying, 'refused_send'::character varying, 'manual'::character varying])::text[]));
