-- +goose NO TRANSACTION
-- +goose Up
-- Every stored address is in its Email case (ADR 0107): each owner puts the address it writes in
-- it, and the CHECK of every address column refuses a write that skipped it. Every writer has
-- always lower-cased these columns, so the rows already hold the rule; each CHECK is added NOT
-- VALID and validated after, so the table is not locked while it is checked.
-- modify "auth_attempts" table
ALTER TABLE "auth_attempts" ADD CONSTRAINT "auth_attempts_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;
ALTER TABLE "auth_attempts" VALIDATE CONSTRAINT "auth_attempts_email_lower";
-- modify "captured_messages" table
ALTER TABLE "captured_messages" ADD CONSTRAINT "captured_messages_recipient_lower" CHECK ((recipient)::text = lower((recipient)::text)) NOT VALID;
ALTER TABLE "captured_messages" VALIDATE CONSTRAINT "captured_messages_recipient_lower";
-- modify "confirmations" table
ALTER TABLE "confirmations" ADD CONSTRAINT "confirmations_destination_lower" CHECK ((destination)::text = lower((destination)::text)) NOT VALID;
ALTER TABLE "confirmations" VALIDATE CONSTRAINT "confirmations_destination_lower";
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_destination_lower" CHECK ((destination)::text = lower((destination)::text)) NOT VALID;
ALTER TABLE "outbound_messages" VALIDATE CONSTRAINT "outbound_messages_destination_lower";
-- modify "pending_signups" table
ALTER TABLE "pending_signups" ADD CONSTRAINT "pending_signups_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;
ALTER TABLE "pending_signups" VALIDATE CONSTRAINT "pending_signups_email_lower";
-- modify "staff_members" table
ALTER TABLE "staff_members" ADD CONSTRAINT "staff_members_email_lower" CHECK ((email)::text = lower((email)::text)) NOT VALID;
ALTER TABLE "staff_members" VALIDATE CONSTRAINT "staff_members_email_lower";
-- modify "suppressions" table
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_destination_lower" CHECK ((destination)::text = lower((destination)::text)) NOT VALID;
ALTER TABLE "suppressions" VALIDATE CONSTRAINT "suppressions_destination_lower";
-- modify "unsubscribes" table
ALTER TABLE "unsubscribes" ADD CONSTRAINT "unsubscribes_destination_lower" CHECK ((destination)::text = lower((destination)::text)) NOT VALID;
ALTER TABLE "unsubscribes" VALIDATE CONSTRAINT "unsubscribes_destination_lower";

-- +goose Down
-- reverse: modify "unsubscribes" table
ALTER TABLE "unsubscribes" DROP CONSTRAINT "unsubscribes_destination_lower";
-- reverse: modify "suppressions" table
ALTER TABLE "suppressions" DROP CONSTRAINT "suppressions_destination_lower";
-- reverse: modify "staff_members" table
ALTER TABLE "staff_members" DROP CONSTRAINT "staff_members_email_lower";
-- reverse: modify "pending_signups" table
ALTER TABLE "pending_signups" DROP CONSTRAINT "pending_signups_email_lower";
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_destination_lower";
-- reverse: modify "confirmations" table
ALTER TABLE "confirmations" DROP CONSTRAINT "confirmations_destination_lower";
-- reverse: modify "captured_messages" table
ALTER TABLE "captured_messages" DROP CONSTRAINT "captured_messages_recipient_lower";
-- reverse: modify "auth_attempts" table
ALTER TABLE "auth_attempts" DROP CONSTRAINT "auth_attempts_email_lower";
