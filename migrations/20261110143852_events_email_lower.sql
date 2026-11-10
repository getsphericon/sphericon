-- +goose NO TRANSACTION
-- +goose Up
-- An Event's address is in its Email case (ADR 0107): the Event store puts the address it writes in
-- it, and the CHECK refuses a write that skipped it. Events recorded before kept the address as it
-- was sent, so the CHECK is added NOT VALID: it holds for every write from now on and leaves the
-- rows already stored as they are, which a migration does not rewrite (a migration is fast DDL).
-- modify "events" table
ALTER TABLE "events" ADD CONSTRAINT "events_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;

-- +goose Down
-- reverse: modify "events" table
ALTER TABLE "events" DROP CONSTRAINT "events_email_lower";
