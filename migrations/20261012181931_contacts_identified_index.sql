-- +goose Up
-- create index "contacts_workspace_id_identified" to table: "contacts"
CREATE INDEX "contacts_workspace_id_identified" ON "contacts" ("workspace_id") WHERE ((email IS NOT NULL) AND ((email)::text <> ''::text));

-- +goose Down
-- reverse: create index "contacts_workspace_id_identified" to table: "contacts"
DROP INDEX "contacts_workspace_id_identified";
