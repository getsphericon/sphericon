import { emptyGroup } from '../../components/conditions/condition.ts'
import { draftGuards } from '../../editing/editorSession.ts'
import { siteRulesPublish, siteRulesRevert } from '../../generated/site/sdk.gen.ts'
import type { SiteRuleResource } from '../../generated/site/types.gen.ts'
import { zSiteCreateRuleInput } from '../../generated/site/zod.gen.ts'
import type { ResourceEditOperation } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { RuleValues } from './RuleForm.tsx'

// A Rule's Condition is optional on the wire; the editor always holds one, and the empty group
// matches every Contact. A new Rule has no Actions yet.
const blankRule: RuleValues = { name: '', triggerEvents: [], condition: emptyGroup, actions: [] }

export const ruleSchema = resourceFormSchema(
  zSiteCreateRuleInput.required({ condition: true, actions: true }),
  blankRule,
)

// The editor edits the draft, else the live version (ADR 0065).
export const editedRule = (rule: SiteRuleResource) => ({
  name: rule.name,
  ...(rule.draft ?? rule.live),
})

// Publish and Revert of the draft (ADR 0065), serialized with Save by the editor session.
export const ruleOperations = {
  PUBLISH: {
    request: async ({ subject: { path }, signal }) => {
      const { data } = await siteRulesPublish({ path, signal, throwOnError: true })
      return data
    },
    available: draftGuards.publishable,
    texts: (t) => ({
      accepted: t(($) => $.notifications.rulePublished),
      failedTitle: t(($) => $.alerts.ruleStatusErrorTitle),
    }),
  },
  REVERT: {
    request: async ({ subject: { path }, signal }) => {
      const { data } = await siteRulesRevert({ path, signal, throwOnError: true })
      return data
    },
    available: draftGuards.revertible,
    // The one operation that replaces the form: with the live version, which is all that is left.
    replaces: (rule) => ruleSchema.toValues(editedRule(rule)),
    texts: (t) => ({
      accepted: t(($) => $.notifications.ruleReverted),
      failedTitle: t(($) => $.alerts.ruleStatusErrorTitle),
    }),
  },
} satisfies Record<string, ResourceEditOperation<SiteRuleResource, RuleValues>>
