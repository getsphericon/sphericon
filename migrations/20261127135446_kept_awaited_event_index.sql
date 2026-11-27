-- +goose NO TRANSACTION
-- +goose Up
-- The Step records that keep a copy of an awaited Event, by the Event's id (ADR 0136): a refusal of
-- tracking clears the copies of the opens and clicks it deletes by their ids. Built concurrently.
CREATE INDEX CONCURRENTLY "automation_step_records_awaited_event_id" ON "automation_step_records" ("workspace_id", "awaited_event_id") WHERE (awaited_event_id IS NOT NULL);

-- +goose Down
DROP INDEX CONCURRENTLY "automation_step_records_awaited_event_id";
