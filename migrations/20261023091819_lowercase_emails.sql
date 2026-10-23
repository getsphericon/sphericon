-- +goose NO TRANSACTION
-- +goose Up
-- An account address is stored lower-cased (internal/emailcase), so the unique index on users.email
-- holds case-insensitively and an Invitation matches its User. The checks are added NOT VALID and
-- validated apart, so neither holds a lock that stops writes while it scans the table.
-- modify "invitations" table
ALTER TABLE "invitations" ADD CONSTRAINT "invitations_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;
ALTER TABLE "invitations" VALIDATE CONSTRAINT "invitations_email_lower";
-- modify "users" table
ALTER TABLE "users" ADD CONSTRAINT "users_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;
ALTER TABLE "users" VALIDATE CONSTRAINT "users_email_lower";

-- +goose Down
-- reverse: modify "users" table
ALTER TABLE "users" DROP CONSTRAINT "users_email_lower";
-- reverse: modify "invitations" table
ALTER TABLE "invitations" DROP CONSTRAINT "invitations_email_lower";
