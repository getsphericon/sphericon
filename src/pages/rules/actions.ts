import { SiteRuleActionType } from '../../generated/site/types.gen.ts'
import type { SiteRuleAction } from '../../generated/site/types.gen.ts'

// Every kind of Action the editor offers and what it is edited with. A new kind is one more
// entry here, with its own fields rendered by RuleForm.
const actionKinds: Record<
  SiteRuleActionType,
  { takesTag: boolean; takesAutomation: boolean; takesCall: boolean }
> = {
  [SiteRuleActionType.APPLY_TAG]: { takesTag: true, takesAutomation: false, takesCall: false },
  [SiteRuleActionType.REMOVE_TAG]: { takesTag: true, takesAutomation: false, takesCall: false },
  [SiteRuleActionType.ENROLL_AUTOMATION]: {
    takesTag: false,
    takesAutomation: true,
    takesCall: false,
  },
  [SiteRuleActionType.HTTP]: { takesTag: false, takesAutomation: false, takesCall: true },
}

export const ruleActionKinds = Object.values(SiteRuleActionType)

export function takesTag(type: SiteRuleActionType): boolean {
  return actionKinds[type].takesTag
}

export function takesAutomation(type: SiteRuleActionType): boolean {
  return actionKinds[type].takesAutomation
}

export function takesCall(type: SiteRuleActionType): boolean {
  return actionKinds[type].takesCall
}

export function newAction(): SiteRuleAction {
  return { type: SiteRuleActionType.APPLY_TAG, tag: '' }
}
