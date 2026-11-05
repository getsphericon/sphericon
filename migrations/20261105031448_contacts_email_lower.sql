-- +goose NO TRANSACTION
-- +goose Up
-- A Contact's address is stored lower-cased, like a User's (internal/emailcase). The contacts
-- module has always lower-cased it, so the rows already hold the rule; the CHECK is added NOT
-- VALID and validated after, so the table is not locked while it is checked.
-- modify "contacts" table
ALTER TABLE "contacts" ADD CONSTRAINT "contacts_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;
ALTER TABLE "contacts" VALIDATE CONSTRAINT "contacts_email_lower";

-- +goose Down
-- reverse: modify "contacts" table
ALTER TABLE "contacts" DROP CONSTRAINT "contacts_email_lower";
