-- +goose Up
-- modify "broadcasts" table
ALTER TABLE "broadcasts" ADD COLUMN "discarded_at" timestamptz NULL;
-- modify "workspaces" table: deleted_at is renamed, never dropped, so the deletion windows
-- of Workspaces already in the cycle (ADR 0043) carry over. The guard (ADR 0033) refuses to
-- rename onto a discarded_at column something else made: its values would shadow the windows.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'workspaces' AND column_name = 'discarded_at'
  ) THEN
    RAISE EXCEPTION 'workspaces.deleted_at cannot be renamed: workspaces.discarded_at already exists, so the deletion windows would not carry over; drop or move that column first';
  END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE "workspaces" RENAME COLUMN "deleted_at" TO "discarded_at";

-- +goose Down
-- reverse: modify "workspaces" table
ALTER TABLE "workspaces" RENAME COLUMN "discarded_at" TO "deleted_at";
-- reverse: modify "broadcasts" table
ALTER TABLE "broadcasts" DROP COLUMN "discarded_at";
