-- +goose NO TRANSACTION
-- +goose Up
-- contact_tags carries the Workspace of its Contact and its Tag (ADR 0017): both its references
-- become composite keys, so the database refuses a Tag of one Workspace on a Contact of another.
-- The column is added nullable, a NOT VALID CHECK refuses a new row without it, and the rows are
-- filled from their Contact; the CHECK is validated before SET NOT NULL, so no scan runs under an
-- exclusive lock. A row joining two Workspaces was never valid and is dropped. Indexes are built
-- first and dropped last, concurrently; each key is added NOT VALID and validated after.
CREATE UNIQUE INDEX CONCURRENTLY "tags_workspace_id_id" ON "tags" ("workspace_id", "id");
ALTER TABLE "contact_tags" ADD COLUMN "workspace_id" bigint NULL;
ALTER TABLE "contact_tags" ADD CONSTRAINT "contact_tags_workspace_id_not_null" CHECK ("workspace_id" IS NOT NULL) NOT VALID;
UPDATE "contact_tags" AS ct SET "workspace_id" = c."workspace_id" FROM "contacts" AS c WHERE c."id" = ct."contact_id";
DELETE FROM "contact_tags" AS ct USING "tags" AS t WHERE t."id" = ct."tag_id" AND t."workspace_id" <> ct."workspace_id";
ALTER TABLE "contact_tags" VALIDATE CONSTRAINT "contact_tags_workspace_id_not_null";
ALTER TABLE "contact_tags" ALTER COLUMN "workspace_id" SET NOT NULL;
ALTER TABLE "contact_tags" DROP CONSTRAINT "contact_tags_workspace_id_not_null";
CREATE INDEX CONCURRENTLY "contacttag_workspace_id_contact_id" ON "contact_tags" ("workspace_id", "contact_id");
CREATE INDEX CONCURRENTLY "contacttag_workspace_id_tag_id" ON "contact_tags" ("workspace_id", "tag_id");
ALTER TABLE "contact_tags" ADD CONSTRAINT "contact_tags_workspaces_contact_tags" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION NOT VALID;
ALTER TABLE "contact_tags" VALIDATE CONSTRAINT "contact_tags_workspaces_contact_tags";
ALTER TABLE "contact_tags" DROP CONSTRAINT "contact_tags_contact_id", ADD CONSTRAINT "contact_tags_contact_id" FOREIGN KEY ("workspace_id", "contact_id") REFERENCES "contacts" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "contact_tags" VALIDATE CONSTRAINT "contact_tags_contact_id";
ALTER TABLE "contact_tags" DROP CONSTRAINT "contact_tags_tag_id", ADD CONSTRAINT "contact_tags_tag_id" FOREIGN KEY ("workspace_id", "tag_id") REFERENCES "tags" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE CASCADE NOT VALID;
ALTER TABLE "contact_tags" VALIDATE CONSTRAINT "contact_tags_tag_id";
DROP INDEX CONCURRENTLY "contacttag_tag_id";

-- +goose Down
DROP INDEX CONCURRENTLY "contacttag_workspace_id_contact_id";
CREATE INDEX CONCURRENTLY "contacttag_tag_id" ON "contact_tags" ("tag_id");
ALTER TABLE "contact_tags" DROP CONSTRAINT "contact_tags_workspaces_contact_tags", DROP CONSTRAINT "contact_tags_tag_id", DROP CONSTRAINT "contact_tags_contact_id", DROP COLUMN "workspace_id", ADD CONSTRAINT "contact_tags_tag_id" FOREIGN KEY ("tag_id") REFERENCES "tags" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, ADD CONSTRAINT "contact_tags_contact_id" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE CASCADE;
DROP INDEX CONCURRENTLY "tags_workspace_id_id";
