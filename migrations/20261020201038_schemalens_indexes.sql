-- +goose NO TRANSACTION
-- +goose Up
-- create index "apitoken_workspace_id" to table: "api_tokens"
CREATE INDEX CONCURRENTLY "apitoken_workspace_id" ON "api_tokens" ("workspace_id");
-- create index "automationrun_workspace_id" to table: "automation_runs"
CREATE INDEX CONCURRENTLY "automationrun_workspace_id" ON "automation_runs" ("workspace_id");
-- drop index "automation_workspace_id" from table: "automations"
DROP INDEX CONCURRENTLY "automation_workspace_id";
-- drop index "broadcastrecipient_broadcast_id" from table: "broadcast_recipients"
DROP INDEX CONCURRENTLY "broadcastrecipient_broadcast_id";
-- create index "broadcastrecipient_workspace_id" to table: "broadcast_recipients"
CREATE INDEX CONCURRENTLY "broadcastrecipient_workspace_id" ON "broadcast_recipients" ("workspace_id");
-- drop index "custom_fields_key_workspace_id" from table: "custom_fields"
DROP INDEX CONCURRENTLY "custom_fields_key_workspace_id";
-- create index "custom_fields_workspace_id_key" to table: "custom_fields"
CREATE UNIQUE INDEX CONCURRENTLY "custom_fields_workspace_id_key" ON "custom_fields" ("workspace_id", "key");
-- create index "event_contact_id" to table: "events"
CREATE INDEX CONCURRENTLY "event_contact_id" ON "events" ("contact_id");
-- drop index "holds_key_workspace_id" from table: "holds"
DROP INDEX CONCURRENTLY "holds_key_workspace_id";
-- create index "holds_workspace_id_key" to table: "holds"
CREATE UNIQUE INDEX CONCURRENTLY "holds_workspace_id_key" ON "holds" ("workspace_id", "key");
-- create index "integration_workspace_id" to table: "integrations"
CREATE INDEX CONCURRENTLY "integration_workspace_id" ON "integrations" ("workspace_id");
-- create index "invitation_invited_by" to table: "invitations"
CREATE INDEX CONCURRENTLY "invitation_invited_by" ON "invitations" ("invited_by");
-- create index "membership_workspace_id" to table: "memberships"
CREATE INDEX CONCURRENTLY "membership_workspace_id" ON "memberships" ("workspace_id");
-- create index "oauthcode_client_id" to table: "oauth_codes"
CREATE INDEX CONCURRENTLY "oauthcode_client_id" ON "oauth_codes" ("client_id");
-- create index "oauthrefreshtoken_client_id" to table: "oauth_refresh_tokens"
CREATE INDEX CONCURRENTLY "oauthrefreshtoken_client_id" ON "oauth_refresh_tokens" ("client_id");
-- create index "segment_workspace_id" to table: "segments"
CREATE INDEX CONCURRENTLY "segment_workspace_id" ON "segments" ("workspace_id");
-- create index "sendlimiter_workspace_id" to table: "send_limiters"
CREATE INDEX CONCURRENTLY "sendlimiter_workspace_id" ON "send_limiters" ("workspace_id");
-- drop index "tags_name_workspace_id" from table: "tags"
DROP INDEX CONCURRENTLY "tags_name_workspace_id";
-- create index "tags_workspace_id_name" to table: "tags"
CREATE UNIQUE INDEX CONCURRENTLY "tags_workspace_id_name" ON "tags" ("workspace_id", "name");
-- drop index "unsubscribes_ws_channel_dest" from table: "unsubscribes"
DROP INDEX CONCURRENTLY "unsubscribes_ws_channel_dest";
-- drop index "visitors_visitor_id_workspace_id" from table: "visitors"
DROP INDEX CONCURRENTLY "visitors_visitor_id_workspace_id";
-- create index "visitor_contact_id" to table: "visitors"
CREATE INDEX CONCURRENTLY "visitor_contact_id" ON "visitors" ("contact_id");
-- create index "visitors_workspace_id_visitor_id" to table: "visitors"
CREATE UNIQUE INDEX CONCURRENTLY "visitors_workspace_id_visitor_id" ON "visitors" ("workspace_id", "visitor_id");
-- drop index "webhookendpoint_workspace_id" from table: "webhook_endpoints"
DROP INDEX CONCURRENTLY "webhookendpoint_workspace_id";

-- +goose Down
-- reverse: drop index "webhookendpoint_workspace_id" from table: "webhook_endpoints"
CREATE INDEX CONCURRENTLY "webhookendpoint_workspace_id" ON "webhook_endpoints" ("workspace_id");
-- reverse: create index "visitors_workspace_id_visitor_id" to table: "visitors"
DROP INDEX CONCURRENTLY "visitors_workspace_id_visitor_id";
-- reverse: create index "visitor_contact_id" to table: "visitors"
DROP INDEX CONCURRENTLY "visitor_contact_id";
-- reverse: drop index "visitors_visitor_id_workspace_id" from table: "visitors"
CREATE UNIQUE INDEX CONCURRENTLY "visitors_visitor_id_workspace_id" ON "visitors" ("visitor_id", "workspace_id");
-- reverse: drop index "unsubscribes_ws_channel_dest" from table: "unsubscribes"
CREATE INDEX CONCURRENTLY "unsubscribes_ws_channel_dest" ON "unsubscribes" ("workspace_id", "channel", "destination");
-- reverse: create index "tags_workspace_id_name" to table: "tags"
DROP INDEX CONCURRENTLY "tags_workspace_id_name";
-- reverse: drop index "tags_name_workspace_id" from table: "tags"
CREATE UNIQUE INDEX CONCURRENTLY "tags_name_workspace_id" ON "tags" ("name", "workspace_id");
-- reverse: create index "sendlimiter_workspace_id" to table: "send_limiters"
DROP INDEX CONCURRENTLY "sendlimiter_workspace_id";
-- reverse: create index "segment_workspace_id" to table: "segments"
DROP INDEX CONCURRENTLY "segment_workspace_id";
-- reverse: create index "oauthrefreshtoken_client_id" to table: "oauth_refresh_tokens"
DROP INDEX CONCURRENTLY "oauthrefreshtoken_client_id";
-- reverse: create index "oauthcode_client_id" to table: "oauth_codes"
DROP INDEX CONCURRENTLY "oauthcode_client_id";
-- reverse: create index "membership_workspace_id" to table: "memberships"
DROP INDEX CONCURRENTLY "membership_workspace_id";
-- reverse: create index "invitation_invited_by" to table: "invitations"
DROP INDEX CONCURRENTLY "invitation_invited_by";
-- reverse: create index "integration_workspace_id" to table: "integrations"
DROP INDEX CONCURRENTLY "integration_workspace_id";
-- reverse: create index "holds_workspace_id_key" to table: "holds"
DROP INDEX CONCURRENTLY "holds_workspace_id_key";
-- reverse: drop index "holds_key_workspace_id" from table: "holds"
CREATE UNIQUE INDEX CONCURRENTLY "holds_key_workspace_id" ON "holds" ("key", "workspace_id");
-- reverse: create index "event_contact_id" to table: "events"
DROP INDEX CONCURRENTLY "event_contact_id";
-- reverse: create index "custom_fields_workspace_id_key" to table: "custom_fields"
DROP INDEX CONCURRENTLY "custom_fields_workspace_id_key";
-- reverse: drop index "custom_fields_key_workspace_id" from table: "custom_fields"
CREATE UNIQUE INDEX CONCURRENTLY "custom_fields_key_workspace_id" ON "custom_fields" ("key", "workspace_id");
-- reverse: create index "broadcastrecipient_workspace_id" to table: "broadcast_recipients"
DROP INDEX CONCURRENTLY "broadcastrecipient_workspace_id";
-- reverse: drop index "broadcastrecipient_broadcast_id" from table: "broadcast_recipients"
CREATE INDEX CONCURRENTLY "broadcastrecipient_broadcast_id" ON "broadcast_recipients" ("broadcast_id");
-- reverse: drop index "automation_workspace_id" from table: "automations"
CREATE INDEX CONCURRENTLY "automation_workspace_id" ON "automations" ("workspace_id");
-- reverse: create index "automationrun_workspace_id" to table: "automation_runs"
DROP INDEX CONCURRENTLY "automationrun_workspace_id";
-- reverse: create index "apitoken_workspace_id" to table: "api_tokens"
DROP INDEX CONCURRENTLY "apitoken_workspace_id";
