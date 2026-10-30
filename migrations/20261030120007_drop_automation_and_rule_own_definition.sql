-- +goose NO TRANSACTION
-- +goose Up
-- Destructive (ADR 0033, ADR 0065): the definitions of Automations and Rules live only in their
-- versions. This drops the Automation's own Trigger and Integration, the Rule's own Trigger events
-- (rule_triggers), Condition and Actions, the Firing's copy of the Actions, and the draft status
-- of Automations and Rules. The data moved in the migrations before it (the versions, the live and
-- draft pointers, the Firings' versions, the draft status to inactive), so the guard checks that
-- move and refuses rather than strand a definition. ent's diff wrote the column drops, the index
-- and the required Trigger of a version; the guard, the table drop, the status constraints and the
-- CHECK that lets the Trigger become required without a scan are written by hand, and the index is
-- built before the drop that removes the old one, concurrently (hence no transaction).
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM "automations" WHERE "live_version_id" IS NULL AND "draft_version_id" IS NULL) THEN
    RAISE EXCEPTION 'automations.trigger_event cannot be dropped: an Automation has neither a live nor a draft version, so its definition has not moved to its versions; install the release that moves it (ADR 0065), migrate, then upgrade again';
  END IF;
  IF EXISTS (SELECT 1 FROM "automation_versions" WHERE "trigger_event" IS NULL) THEN
    RAISE EXCEPTION 'automations.trigger_event cannot be dropped: an Automation version has no Trigger, so the Trigger and Integration have not moved to the versions; install the release that moves them (ADR 0065), migrate, then upgrade again';
  END IF;
  IF EXISTS (SELECT 1 FROM "rules" WHERE "live_version_id" IS NULL AND "draft_version_id" IS NULL) THEN
    RAISE EXCEPTION 'rules.condition cannot be dropped: a Rule has neither a live nor a draft version, so its Trigger events, Condition and Actions have not moved to its versions; install the release that moves them (ADR 0065), migrate, then upgrade again';
  END IF;
  IF EXISTS (SELECT 1 FROM "rule_firings" WHERE "version_id" IS NULL) THEN
    RAISE EXCEPTION 'rule_firings.actions cannot be dropped: a Rule Firing names no Rule version, so its Actions would be lost; install the release that moves them (ADR 0065), migrate, then upgrade again';
  END IF;
  IF EXISTS (SELECT 1 FROM "automations" WHERE "status" = 'draft') OR EXISTS (SELECT 1 FROM "rules" WHERE "status" = 'draft') THEN
    RAISE EXCEPTION 'the draft status cannot be dropped: an Automation or a Rule is still draft; install the release that makes it inactive (ADR 0065), migrate, then upgrade again';
  END IF;
END
$$;
-- +goose StatementEnd
-- create index "automation_workspace_id" to table: "automations"
CREATE INDEX CONCURRENTLY "automation_workspace_id" ON "automations" ("workspace_id");
-- modify "automations" table
ALTER TABLE "automations" DROP COLUMN "trigger_event", DROP COLUMN "integration_id";
-- modify "rule_firings" table
ALTER TABLE "rule_firings" DROP COLUMN "actions";
-- modify "rules" table
ALTER TABLE "rules" DROP COLUMN "condition", DROP COLUMN "actions";
DROP TABLE "rule_triggers";
-- The status of an Automation and of a Rule is active or inactive.
ALTER TABLE "automations" DROP CONSTRAINT "automations_status_check";
ALTER TABLE "automations" ADD CONSTRAINT "automations_status_check" CHECK ((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying])::text[])) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_status_check";
ALTER TABLE "rules" DROP CONSTRAINT "rules_status_check";
ALTER TABLE "rules" ADD CONSTRAINT "rules_status_check" CHECK ((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying])::text[])) NOT VALID;
ALTER TABLE "rules" VALIDATE CONSTRAINT "rules_status_check";
-- Every Automation version has a Trigger: the guard above refused a version without one. The
-- CHECK, validated without blocking writes, lets SET NOT NULL skip its own scan of the table.
ALTER TABLE "automation_versions" ADD CONSTRAINT "automation_versions_trigger_event_present" CHECK ("trigger_event" IS NOT NULL) NOT VALID;
ALTER TABLE "automation_versions" VALIDATE CONSTRAINT "automation_versions_trigger_event_present";
ALTER TABLE "automation_versions" ALTER COLUMN "trigger_event" SET NOT NULL;
ALTER TABLE "automation_versions" DROP CONSTRAINT "automation_versions_trigger_event_present";

-- +goose Down
ALTER TABLE "automation_versions" ALTER COLUMN "trigger_event" DROP NOT NULL;
-- The dropped columns and rule_triggers come back filled from the latest version, the draft when
-- there is one, as before ADR 0065.
ALTER TABLE "rules" DROP CONSTRAINT "rules_status_check";
ALTER TABLE "rules" ADD CONSTRAINT "rules_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying, 'inactive'::character varying])::text[])) NOT VALID;
ALTER TABLE "rules" VALIDATE CONSTRAINT "rules_status_check";
ALTER TABLE "automations" DROP CONSTRAINT "automations_status_check";
ALTER TABLE "automations" ADD CONSTRAINT "automations_status_check" CHECK ((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying, 'inactive'::character varying])::text[])) NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_status_check";
CREATE TABLE "rule_triggers" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP, "updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP, "action" character varying NOT NULL, "rule_id" bigint NOT NULL, "workspace_id" bigint NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "rule_triggers_rules_triggers" FOREIGN KEY ("rule_id") REFERENCES "rules" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "rule_triggers_workspaces_rule_triggers" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
CREATE UNIQUE INDEX CONCURRENTLY "ruletrigger_rule_id_action" ON "rule_triggers" ("rule_id", "action");
CREATE INDEX CONCURRENTLY "ruletrigger_workspace_id_action" ON "rule_triggers" ("workspace_id", "action");
INSERT INTO "rule_triggers" ("workspace_id", "rule_id", "action")
SELECT t."workspace_id", r."id", t."action"
FROM "rules" r
JOIN "rule_version_triggers" t ON t."version_id" = COALESCE(r."draft_version_id", r."live_version_id");
-- reverse: modify "rules" table
ALTER TABLE "rules" ADD COLUMN "actions" character varying NOT NULL DEFAULT '[]', ADD COLUMN "condition" character varying NULL;
UPDATE "rules" r SET "condition" = v."condition", "actions" = v."actions"
FROM "rule_versions" v
WHERE v."id" = COALESCE(r."draft_version_id", r."live_version_id");
-- reverse: modify "rule_firings" table
ALTER TABLE "rule_firings" ADD COLUMN "actions" character varying NOT NULL DEFAULT '[]';
UPDATE "rule_firings" f SET "actions" = v."actions"
FROM "rule_versions" v
WHERE v."id" = f."version_id";
-- reverse: modify "automations" table
ALTER TABLE "automations" ADD COLUMN "integration_id" bigint NULL, ADD COLUMN "trigger_event" character varying NULL;
UPDATE "automations" a SET "trigger_event" = v."trigger_event", "integration_id" = v."integration_id"
FROM "automation_versions" v
WHERE v."id" = COALESCE(a."draft_version_id", a."live_version_id");
ALTER TABLE "automations" ADD CONSTRAINT "automations_integrations_integration" FOREIGN KEY ("integration_id") REFERENCES "integrations" ("id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
ALTER TABLE "automations" VALIDATE CONSTRAINT "automations_integrations_integration";
CREATE INDEX CONCURRENTLY "automation_integration_id" ON "automations" ("integration_id");
CREATE INDEX CONCURRENTLY "automation_workspace_id_status_trigger_event" ON "automations" ("workspace_id", "status", "trigger_event");
-- reverse: create index "automation_workspace_id" to table: "automations"
DROP INDEX CONCURRENTLY "automation_workspace_id";
