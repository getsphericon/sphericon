-- +goose Up
-- Breaking (ADR 0033, ADR 0100): an Automation is archived, never discarded, so its discarded_at
-- becomes archived_at, and the Audit log's past discard and restore of an Automation read archive
-- and unarchive, as the new entries do. ent's diff wrote a drop and an add, which would lose every
-- archived Automation; the rename and the rewrite are written by hand.
-- modify "automations" table
ALTER TABLE "automations" RENAME COLUMN "discarded_at" TO "archived_at";
-- rewrite the Audit entries of an Automation's archive and unarchive
UPDATE "audit_entries" SET "action" = 'automation.archive' WHERE "action" = 'automation.discard';
UPDATE "audit_entries" SET "action" = 'automation.unarchive' WHERE "action" = 'automation.restore';

-- +goose Down
-- reverse: rewrite the Audit entries of an Automation's archive and unarchive
UPDATE "audit_entries" SET "action" = 'automation.restore' WHERE "action" = 'automation.unarchive';
UPDATE "audit_entries" SET "action" = 'automation.discard' WHERE "action" = 'automation.archive';
-- reverse: modify "automations" table
ALTER TABLE "automations" RENAME COLUMN "archived_at" TO "discarded_at";
