-- +goose NO TRANSACTION
-- +goose Up
-- A key its owner normalises is stored only in the normalised form: a Contact's phone in E.164 and its
-- country as an ISO 3166-1 alpha-2 code, a Join, Sending or Tracking domain as lower-case punycode.
-- Each pattern is the normaliser's own. Every writer normalises already, so each CHECK is added NOT
-- VALID and validated after, and the table is not locked while it is checked.
-- modify "contacts" table
ALTER TABLE "contacts" ADD CONSTRAINT "contacts_country_format" CHECK ((country)::text ~ '^[A-Z]{2}$'::text) NOT VALID;
ALTER TABLE "contacts" VALIDATE CONSTRAINT "contacts_country_format";
ALTER TABLE "contacts" ADD CONSTRAINT "contacts_phone_format" CHECK ((phone)::text ~ '^\+[1-9]\d{1,14}$'::text) NOT VALID;
ALTER TABLE "contacts" VALIDATE CONSTRAINT "contacts_phone_format";
-- modify "join_domains" table
ALTER TABLE "join_domains" ADD CONSTRAINT "join_domains_domain_format" CHECK ((domain)::text ~ '^[a-z0-9-]+(\.[a-z0-9-]+)+$'::text) NOT VALID;
ALTER TABLE "join_domains" VALIDATE CONSTRAINT "join_domains_domain_format";
-- modify "sending_domains" table
ALTER TABLE "sending_domains" ADD CONSTRAINT "sending_domains_domain_format" CHECK ((domain)::text ~ '^[a-z0-9-]+(\.[a-z0-9-]+)+$'::text) NOT VALID;
ALTER TABLE "sending_domains" VALIDATE CONSTRAINT "sending_domains_domain_format";
-- modify "tracking_domains" table
ALTER TABLE "tracking_domains" ADD CONSTRAINT "tracking_domains_host_format" CHECK ((host)::text ~ '^[a-z0-9-]+(\.[a-z0-9-]+)+$'::text) NOT VALID;
ALTER TABLE "tracking_domains" VALIDATE CONSTRAINT "tracking_domains_host_format";

-- +goose Down
-- reverse: modify "tracking_domains" table
ALTER TABLE "tracking_domains" DROP CONSTRAINT "tracking_domains_host_format";
-- reverse: modify "sending_domains" table
ALTER TABLE "sending_domains" DROP CONSTRAINT "sending_domains_domain_format";
-- reverse: modify "join_domains" table
ALTER TABLE "join_domains" DROP CONSTRAINT "join_domains_domain_format";
-- reverse: modify "contacts" table
ALTER TABLE "contacts" DROP CONSTRAINT "contacts_phone_format", DROP CONSTRAINT "contacts_country_format";
