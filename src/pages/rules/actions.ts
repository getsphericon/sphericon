import { pick } from 'es-toolkit'

import { RuleActionType } from '../../generated/site/types.gen.ts'
import type { RuleAction, RuleActionInput } from '../../generated/site/types.gen.ts'

// Every kind of Action the editor offers and the fields of the contract's flat Action it takes
// beside its type (the contract's description of each field names its kinds). A new kind is one
// more entry here, with its own fields rendered by RuleForm.
const actionFields: Record<RuleActionType, readonly (keyof RuleActionInput)[]> = {
  [RuleActionType.APPLY_TAG]: ['tag'],
  [RuleActionType.REMOVE_TAG]: ['tag'],
  [RuleActionType.ENROLL_AUTOMATION]: ['automationId'],
  [RuleActionType.HTTP]: ['id', 'url', 'headers', 'body', 'secret', 'clearSecret', 'hasSecret'],
}

export const ruleActionKinds = Object.values(RuleActionType)

export function takesTag(type: RuleActionType): boolean {
  return actionFields[type].includes('tag')
}

export function takesAutomation(type: RuleActionType): boolean {
  return actionFields[type].includes('automationId')
}

export function takesCall(type: RuleActionType): boolean {
  return actionFields[type].includes('url')
}

// ofItsKind is the Action with its own kind's fields alone: one switched to another kind in the
// form still holds what it was given as the kind it was, which its kind does not send.
export function ofItsKind(action: RuleActionInput): RuleActionInput {
  return { ...pick(action, actionFields[action.type]), type: action.type }
}

export function newAction(): RuleAction {
  return { type: RuleActionType.APPLY_TAG, tag: '' }
}
