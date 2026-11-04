-- +goose Up
-- Breaking (ADR 0033, ADR 0100): a Broadcast is archived, never discarded, so its discarded_at
-- becomes archived_at, and the Audit log's past discard and restore of a Broadcast read archive
-- and unarchive, as the new entries do. ent's diff wrote a drop and an add, which would lose every
-- archived Broadcast; the rename and the rewrite are written by hand.
-- modify "broadcasts" table
ALTER TABLE "broadcasts" RENAME COLUMN "discarded_at" TO "archived_at";
-- rewrite the Audit entries of a Broadcast's archive and unarchive
UPDATE "audit_entries" SET "action" = 'broadcast.archive' WHERE "action" = 'broadcast.discard';
UPDATE "audit_entries" SET "action" = 'broadcast.unarchive' WHERE "action" = 'broadcast.restore';

-- +goose Down
-- reverse: rewrite the Audit entries of a Broadcast's archive and unarchive
UPDATE "audit_entries" SET "action" = 'broadcast.restore' WHERE "action" = 'broadcast.unarchive';
UPDATE "audit_entries" SET "action" = 'broadcast.discard' WHERE "action" = 'broadcast.archive';
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" RENAME COLUMN "archived_at" TO "discarded_at";
