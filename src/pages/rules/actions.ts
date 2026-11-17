import { RuleActionType } from '../../generated/site/types.gen.ts'
import type { RuleAction } from '../../generated/site/types.gen.ts'

// Every kind of Action the editor offers and what it is edited with. A new kind is one more
// entry here, with its own fields rendered by RuleForm.
const actionKinds: Record<
  RuleActionType,
  { takesTag: boolean; takesAutomation: boolean; takesCall: boolean }
> = {
  [RuleActionType.APPLY_TAG]: { takesTag: true, takesAutomation: false, takesCall: false },
  [RuleActionType.REMOVE_TAG]: { takesTag: true, takesAutomation: false, takesCall: false },
  [RuleActionType.ENROLL_AUTOMATION]: {
    takesTag: false,
    takesAutomation: true,
    takesCall: false,
  },
  [RuleActionType.HTTP]: { takesTag: false, takesAutomation: false, takesCall: true },
}

export const ruleActionKinds = Object.values(RuleActionType)

export function takesTag(type: RuleActionType): boolean {
  return actionKinds[type].takesTag
}

export function takesAutomation(type: RuleActionType): boolean {
  return actionKinds[type].takesAutomation
}

export function takesCall(type: RuleActionType): boolean {
  return actionKinds[type].takesCall
}

export function newAction(): RuleAction {
  return { type: RuleActionType.APPLY_TAG, tag: '' }
}
