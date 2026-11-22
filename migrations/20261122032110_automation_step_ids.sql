-- +goose Up
-- An Automation message names the version and the Step id that sent it instead of the Step's
-- position (ADR 0122): a Step keeps its id in every version, so a Step inserted above it in a later
-- version moves neither its report nor its idempotency key. Each message gets them through its run:
-- the run's version and the Step at the message's position in it, and its key names the Step by
-- that id. A message whose run is gone (an Erasure) or whose position its version does not have
-- names no version and goes. The pruned figures name no version either and go. The constraints are
-- added NOT VALID under the runner's bounded lock wait, and validated by the next migration
-- (20261122032111), which builds the indexes concurrently.
-- modify "outbound_messages" table
ALTER TABLE "outbound_messages" ADD COLUMN "automation_step_id" character varying NULL, ADD COLUMN "automation_version_id" bigint NULL;
UPDATE "outbound_messages" AS m
SET "automation_version_id" = r."version_id",
    "automation_step_id" = s.step ->> 'id',
    "idempotency_key" = 'automation:' || r."id" || ':' || (s.step ->> 'id')
FROM "automation_runs" AS r
JOIN "automation_versions" AS v ON v."workspace_id" = r."workspace_id" AND v."id" = r."version_id"
CROSS JOIN LATERAL jsonb_array_elements(v."steps" -> 'steps') WITH ORDINALITY AS s(step, ord)
WHERE m."kind" = 'automation'
  AND r."workspace_id" = m."workspace_id"
  AND r."id" = m."automation_run_id"
  AND s.ord = m."automation_step" + 1;
DELETE FROM "outbound_messages" WHERE "kind" = 'automation' AND "automation_step_id" IS NULL;
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_provenance_check";
ALTER TABLE "outbound_messages" DROP COLUMN "automation_step";
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_provenance_check" CHECK ((kind = 'broadcast' AND broadcast_id IS NOT NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL AND template_id IS NULL) OR (kind = 'automation' AND automation_id IS NOT NULL AND automation_version_id IS NOT NULL AND automation_step_id IS NOT NULL AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND template_id IS NULL) OR (kind = 'transactional' AND broadcast_id IS NULL AND broadcast_recipient_id IS NULL AND automation_id IS NULL AND automation_version_id IS NULL AND automation_step_id IS NULL AND automation_run_id IS NULL)) NOT VALID;
ALTER TABLE "outbound_messages" ADD CONSTRAINT "outbound_messages_automation_versions_automation_version" FOREIGN KEY ("workspace_id", "automation_version_id") REFERENCES "automation_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE RESTRICT NOT VALID;
-- modify "automation_figures" table
DELETE FROM "automation_figures";
ALTER TABLE "automation_figures" DROP COLUMN "step", ADD COLUMN "step_id" character varying NOT NULL, ADD COLUMN "version_id" bigint NOT NULL;
ALTER TABLE "automation_figures" ADD CONSTRAINT "automation_figures_automation_versions_version" FOREIGN KEY ("workspace_id", "version_id") REFERENCES "automation_versions" ("workspace_id", "id") ON UPDATE NO ACTION ON DELETE NO ACTION NOT VALID;

-- +goose Down
-- reverse: modify "automation_figures" table
ALTER TABLE "automation_figures" DROP CONSTRAINT "automation_figures_automation_versions_version", DROP COLUMN "version_id", DROP COLUMN "step_id", ADD COLUMN "step" bigint NOT NULL;
CREATE UNIQUE INDEX "automation_figures_workspace_id_automation_id_step" ON "automation_figures" ("workspace_id", "automation_id", "step");
-- reverse: modify "outbound_messages" table
ALTER TABLE "outbound_messages" DROP CONSTRAINT "outbound_messages_automation_versions_automation_version", DROP CONSTRAINT "outbound_messages_provenance_check", DROP COLUMN "automation_version_id", DROP COLUMN "automation_step_id", ADD COLUMN "automation_step" bigint NULL;
