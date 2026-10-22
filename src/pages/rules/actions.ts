import * as z from 'zod'

import { SiteRuleActionType } from '../../generated/site/types.gen.ts'
import type { SiteRuleAction } from '../../generated/site/types.gen.ts'
import { zSiteRuleAction } from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'

// Every kind of Action the editor offers and what it is edited with. A later kind (a webhook
// call, an enrollment) is one more entry here, with its own fields rendered by RuleForm.
const actionKinds: Record<SiteRuleActionType, { takesTag: boolean }> = {
  [SiteRuleActionType.APPLY_TAG]: { takesTag: true },
  [SiteRuleActionType.REMOVE_TAG]: { takesTag: true },
}

export const ruleActionKinds = Object.values(SiteRuleActionType)

export function takesTag(type: SiteRuleActionType): boolean {
  return actionKinds[type].takesTag
}

export function newAction(): SiteRuleAction {
  return { type: SiteRuleActionType.APPLY_TAG, tag: '' }
}

// The form holds the Actions as JSON text; a blank text is no Actions.
export const actionsCodec: z.ZodType<SiteRuleAction[], string> = jsonText.pipe(
  z.array(zSiteRuleAction),
)

// The form holds the Trigger events as JSON text too. An empty list is held as a blank text,
// so the form reports "required" rather than a length error.
export const triggerEventsCodec: z.ZodType<string[], string> = jsonText.pipe(
  z.array(z.string()).min(1),
)
