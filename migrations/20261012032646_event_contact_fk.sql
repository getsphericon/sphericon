-- +goose Up
-- modify "events" table
ALTER TABLE "events" ADD CONSTRAINT "events_contacts_events" FOREIGN KEY ("contact_id") REFERENCES "contacts" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;

-- +goose Down
-- reverse: modify "events" table
ALTER TABLE "events" DROP CONSTRAINT "events_contacts_events";
