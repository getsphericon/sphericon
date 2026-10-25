import * as z from 'zod'

import { conditionCodec, emptyGroup } from '../../components/conditions/condition.ts'
import {
  siteRulesCreateMutation,
  siteRulesGetOptions,
  siteRulesGetQueryKey,
  siteRulesListQueryKey,
  siteRulesUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteCreateRuleInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { actionsCodec, triggerEventsCodec } from './actions.ts'
import { RuleDryRun } from './RuleDryRun.tsx'
import { RuleForm } from './RuleForm.tsx'

// A Rule's Condition is optional on the wire; the editor always holds one, and the empty group
// matches every Contact. A new Rule has no Actions yet.
const ruleSchema = resourceFormSchema(
  z.object({
    name: zSiteCreateRuleInput.shape.name,
    triggerEvents: triggerEventsCodec,
    condition: conditionCodec.default(emptyGroup),
    actions: actionsCodec.default([]),
  }),
)

const ruleResource = defineResource({
  schema: ruleSchema,
  Form: RuleForm,
  getOptions: siteRulesGetOptions,
  createMutation: siteRulesCreateMutation,
  updateMutation: siteRulesUpdateMutation,
  listQueryKey: siteRulesListQueryKey,
  getQueryKey: siteRulesGetQueryKey,
  editRoute: rulesEditRoute,
  idParam: 'ruleId',
  texts: (t) => ({
    createTitle: t(($) => $.rules.createTitle),
    editTitle: t(($) => $.rules.editTitle),
    created: t(($) => $.notifications.ruleCreated),
    updated: t(($) => $.notifications.ruleUpdated),
    loadErrorTitle: t(($) => $.alerts.ruleLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.ruleSaveErrorTitle),
  }),
  editTarget: (created, { workspaceId }) => ({
    to: rulesEditRoute.to,
    params: { workspaceId, ruleId: created.id },
  }),
})

// Function declarations (hoisted) because the route files and this module import each other.
export function RuleCreatePage() {
  return <ruleResource.CreatePage />
}

export function RuleEditPage() {
  const { workspaceId, ruleId } = rulesEditRoute.useParams()
  return (
    <ruleResource.EditPage>
      {() => <RuleDryRun workspaceId={workspaceId} ruleId={ruleId} />}
    </ruleResource.EditPage>
  )
}
