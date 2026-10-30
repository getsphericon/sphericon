-- +goose Up
-- Every Rule gets its version 1 from the definition it holds now (ADR 0065, #771), archived Rules
-- included: its Condition, its Actions and its Trigger events. A Rule that was ever published
-- (activated_at is set) runs it, so it is live; any other Rule has never run, so it is the draft.
-- Every Firing references version 1 of its Rule: a Firing exists only for a published Rule, so it
-- never lands on a draft. The old columns and rule_triggers stay, unread, until the breaking ticket
-- of spec #767 drops them (ADR 0033). Written by hand: ent's diff writes no data.
INSERT INTO "rule_versions" ("workspace_id", "rule_id", "version", "condition", "actions")
SELECT "workspace_id", "id", 1, "condition", "actions" FROM "rules";
INSERT INTO "rule_version_triggers" ("workspace_id", "version_id", "action")
SELECT t."workspace_id", v."id", t."action"
FROM "rule_triggers" t
JOIN "rule_versions" v ON v."rule_id" = t."rule_id" AND v."version" = 1;
UPDATE "rules" r SET "live_version_id" = v."id"
FROM "rule_versions" v
WHERE v."rule_id" = r."id" AND v."version" = 1 AND r."activated_at" IS NOT NULL;
UPDATE "rules" r SET "draft_version_id" = v."id"
FROM "rule_versions" v
WHERE v."rule_id" = r."id" AND v."version" = 1 AND r."activated_at" IS NULL;
-- One row per Firing: the table is bounded by the Firing retention window (ADR 0019), so this is
-- a fast statement, not heavy work for a river job.
UPDATE "rule_firings" f SET "version_id" = v."id"
FROM "rule_versions" v
WHERE v."rule_id" = f."rule_id" AND v."version" = 1;

-- +goose Down
UPDATE "rule_firings" SET "version_id" = NULL;
UPDATE "rules" SET "live_version_id" = NULL, "draft_version_id" = NULL;
DELETE FROM "rule_version_triggers";
DELETE FROM "rule_versions";
