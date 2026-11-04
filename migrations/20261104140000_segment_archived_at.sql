-- +goose Up
-- A Segment is archived, never discarded (ADR 0100): discarded_at becomes archived_at. ent's diff
-- would drop the column and add a new one, losing every archive, so the rename and the rewrite of
-- the Audit actions below are written by hand. The Audit log names the action as the owner sees it:
-- what a Segment recorded as discard and restore reads archive and unarchive.
-- modify "segments" table
ALTER TABLE "segments" RENAME COLUMN "discarded_at" TO "archived_at";
UPDATE "audit_entries" SET "action" = 'segment.archive' WHERE "action" = 'segment.discard';
UPDATE "audit_entries" SET "action" = 'segment.unarchive' WHERE "action" = 'segment.restore';

-- +goose Down
UPDATE "audit_entries" SET "action" = 'segment.restore' WHERE "action" = 'segment.unarchive';
UPDATE "audit_entries" SET "action" = 'segment.discard' WHERE "action" = 'segment.archive';
-- reverse: modify "segments" table
ALTER TABLE "segments" RENAME COLUMN "archived_at" TO "discarded_at";
