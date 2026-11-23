-- +goose Up
-- An Order's credit names the Outbound message whose click earned it (ADR 0064, ADR 0083, ADR 0120),
-- beside the Broadcast or Automation and the click's time, and the credit check ties the message to
-- them. The project carries no legacy (ADR 0047): no credited Order is carried over, so a database
-- that holds one is refused before the check changes; it is reset (`mise run db:reset`). ent's diff
-- wrote the column and the index but does not see a changed check: the check is replaced by hand,
-- added NOT VALID, and the next migration (20261123192226) validates it and builds the index
-- concurrently.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "orders" WHERE "credited_click_at" IS NOT NULL) THEN
    RAISE EXCEPTION 'the database holds credited Orders that name no clicked message (ADR 0120): reset it, the project carries no legacy (ADR 0047)';
  END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE "orders" ADD COLUMN "credited_message_id" bigint NULL;
ALTER TABLE "orders" DROP CONSTRAINT "orders_credit_check";
ALTER TABLE "orders" ADD CONSTRAINT "orders_credit_check" CHECK (((broadcast_id IS NULL) OR (automation_id IS NULL)) AND ((credited_click_at IS NULL) = ((broadcast_id IS NULL) AND (automation_id IS NULL))) AND ((credited_message_id IS NULL) = ((broadcast_id IS NULL) AND (automation_id IS NULL)))) NOT VALID;

-- +goose Down
ALTER TABLE "orders" DROP CONSTRAINT "orders_credit_check";
ALTER TABLE "orders" ADD CONSTRAINT "orders_credit_check" CHECK (((broadcast_id IS NULL) OR (automation_id IS NULL)) AND ((credited_click_at IS NULL) = ((broadcast_id IS NULL) AND (automation_id IS NULL))));
ALTER TABLE "orders" DROP COLUMN "credited_message_id";
