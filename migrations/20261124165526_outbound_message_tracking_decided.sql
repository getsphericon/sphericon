-- +goose Up
-- When a render decided whether an Outbound message goes out tracked (ADR 0120): a message skipped
-- before its render has decided nothing. A new row's tracked is false until that decision; the
-- rows already there keep true with no moment (every message before went out tracked).
ALTER TABLE "outbound_messages" ALTER COLUMN "tracked" SET DEFAULT false, ADD COLUMN "tracking_decided_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP COLUMN "tracking_decided_at", ALTER COLUMN "tracked" SET DEFAULT true;
