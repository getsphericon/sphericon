-- +goose Up
-- modify "provider_limiters" table
ALTER TABLE "provider_limiters" DROP COLUMN "second_fill", DROP COLUMN "day_fill", ADD COLUMN "second_tokens" double precision NULL, ADD COLUMN "day_tokens" double precision NULL;
-- modify "send_cap_limiters" table
ALTER TABLE "send_cap_limiters" DROP COLUMN "day_fill", ADD COLUMN "day_tokens" double precision NULL;

-- +goose Down
-- reverse: modify "send_cap_limiters" table
ALTER TABLE "send_cap_limiters" DROP COLUMN "day_tokens", ADD COLUMN "day_fill" double precision NOT NULL;
-- reverse: modify "provider_limiters" table
ALTER TABLE "provider_limiters" DROP COLUMN "day_tokens", DROP COLUMN "second_tokens", ADD COLUMN "day_fill" double precision NOT NULL, ADD COLUMN "second_fill" double precision NOT NULL;
