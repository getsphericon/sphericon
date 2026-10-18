-- +goose Up
-- The platform identity is a Staff member now (it was called Operator): its table, the names of
-- the table's objects and the stored values that carried the old word follow.
ALTER TABLE "operators" RENAME TO "staff_members";
ALTER INDEX "operators_pkey" RENAME TO "staff_members_pkey";
ALTER INDEX "operators_email_key" RENAME TO "staff_members_email_key";
ALTER SEQUENCE "operators_id_seq" RENAME TO "staff_members_id_seq";
UPDATE "workspaces" SET "suspended_by_kind" = 'staff' WHERE "suspended_by_kind" = 'operator';
UPDATE "audit_entries" SET "actor_kind" = 'staff' WHERE "actor_kind" = 'operator';
UPDATE "auth_attempts" SET "kind" = 'staff_login' WHERE "kind" = 'operator_login';

-- +goose Down
UPDATE "auth_attempts" SET "kind" = 'operator_login' WHERE "kind" = 'staff_login';
UPDATE "audit_entries" SET "actor_kind" = 'operator' WHERE "actor_kind" = 'staff';
UPDATE "workspaces" SET "suspended_by_kind" = 'operator' WHERE "suspended_by_kind" = 'staff';
ALTER SEQUENCE "staff_members_id_seq" RENAME TO "operators_id_seq";
ALTER INDEX "staff_members_email_key" RENAME TO "operators_email_key";
ALTER INDEX "staff_members_pkey" RENAME TO "operators_pkey";
ALTER TABLE "staff_members" RENAME TO "operators";
