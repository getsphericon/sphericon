-- +goose Up
-- modify "automation_figures" table
ALTER TABLE "automation_figures" ADD COLUMN "skipped_suppressed" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_unsubscribed_everything" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_unsubscribed_source" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_unconfirmed" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_no_destination" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_contact_erased" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_no_local_time" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_frequency_cap" bigint NOT NULL DEFAULT 0;
-- modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" ADD COLUMN "skipped_suppressed" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_unsubscribed_everything" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_unsubscribed_source" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_unconfirmed" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_no_destination" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_contact_erased" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_no_local_time" bigint NOT NULL DEFAULT 0, ADD COLUMN "skipped_frequency_cap" bigint NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "broadcast_figures" table
ALTER TABLE "broadcast_figures" DROP COLUMN "skipped_frequency_cap", DROP COLUMN "skipped_no_local_time", DROP COLUMN "skipped_contact_erased", DROP COLUMN "skipped_no_destination", DROP COLUMN "skipped_unconfirmed", DROP COLUMN "skipped_unsubscribed_source", DROP COLUMN "skipped_unsubscribed_everything", DROP COLUMN "skipped_suppressed";
-- reverse: modify "automation_figures" table
ALTER TABLE "automation_figures" DROP COLUMN "skipped_frequency_cap", DROP COLUMN "skipped_no_local_time", DROP COLUMN "skipped_contact_erased", DROP COLUMN "skipped_no_destination", DROP COLUMN "skipped_unconfirmed", DROP COLUMN "skipped_unsubscribed_source", DROP COLUMN "skipped_unsubscribed_everything", DROP COLUMN "skipped_suppressed";
