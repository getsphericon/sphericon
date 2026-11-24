-- +goose Up
-- An Outbound message's `tracked` is unset until a render decides it (ADR 0120): a message skipped
-- before its render is neither tracked nor untracked. The rows already there keep `true`: every
-- message before the Engagement tracking consent went out tracked.
ALTER TABLE "outbound_messages" ALTER COLUMN "tracked" DROP NOT NULL, ALTER COLUMN "tracked" DROP DEFAULT;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" ALTER COLUMN "tracked" SET NOT NULL, ALTER COLUMN "tracked" SET DEFAULT true;
