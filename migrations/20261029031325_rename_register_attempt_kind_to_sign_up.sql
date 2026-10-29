-- +goose NO TRANSACTION
-- +goose Up
-- The attempt kind "register" is "sign_up" (GLOSSARY: Sign-up). ent's diff does not see a changed
-- enum CHECK, so this migration is written by hand. The stored counters are rewritten in place so a
-- running sign-up budget carries over; auth_attempts holds short-lived throttle rows, purged by age.
ALTER TABLE "auth_attempts" DROP CONSTRAINT "auth_attempts_kind_check";
UPDATE "auth_attempts" SET "kind" = 'sign_up' WHERE "kind" = 'register';
ALTER TABLE "auth_attempts" ADD CONSTRAINT "auth_attempts_kind_check" CHECK ((kind)::text = ANY ((ARRAY['login'::character varying, 'staff_login'::character varying, 'password_reset'::character varying, 'sign_up'::character varying, 'email_change'::character varying, 'test_send_user'::character varying, 'test_send_workspace'::character varying])::text[])) NOT VALID;
ALTER TABLE "auth_attempts" VALIDATE CONSTRAINT "auth_attempts_kind_check";

-- +goose Down
ALTER TABLE "auth_attempts" DROP CONSTRAINT "auth_attempts_kind_check";
UPDATE "auth_attempts" SET "kind" = 'register' WHERE "kind" = 'sign_up';
ALTER TABLE "auth_attempts" ADD CONSTRAINT "auth_attempts_kind_check" CHECK ((kind)::text = ANY ((ARRAY['login'::character varying, 'staff_login'::character varying, 'password_reset'::character varying, 'register'::character varying, 'email_change'::character varying, 'test_send_user'::character varying, 'test_send_workspace'::character varying])::text[])) NOT VALID;
ALTER TABLE "auth_attempts" VALIDATE CONSTRAINT "auth_attempts_kind_check";
