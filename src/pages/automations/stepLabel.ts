import type { TFunction } from 'i18next'

import type { AutomationStep } from '../../generated/site/types.gen.ts'

// A step by its kind, with the subject of an email or the Tag of a tag step.
export function stepLabel(t: TFunction, step: AutomationStep): string {
  const kind: Record<AutomationStep['type'], string> = {
    email: t(($) => $.automations.stepEmail),
    telegram: t(($) => $.automations.stepTelegram),
    sms: t(($) => $.automations.stepSms),
    wait: t(($) => $.automations.stepWait),
    apply_tag: t(($) => $.automations.stepApplyTag),
    remove_tag: t(($) => $.automations.stepRemoveTag),
    http: t(($) => $.automations.stepHttp),
  }
  const detail = stepDetail(step)
  return detail ? `${kind[step.type]}: ${detail}` : kind[step.type]
}

// What a step says beside its kind: an email's subject, a tag step's Tag.
function stepDetail(step: AutomationStep): string {
  switch (step.type) {
    case 'email':
      return step.subject
    case 'apply_tag':
    case 'remove_tag':
      return step.tag
    case 'http':
    case 'sms':
    case 'telegram':
    case 'wait':
    default:
      return ''
  }
}

// A Step of the report named by its id (ADR 0122): its place and label in the live version, or that
// the live version no longer holds it.
export function reportedStepLabel(
  t: TFunction,
  liveSteps: AutomationStep[],
  stepId: string,
): string {
  const place = liveSteps.findIndex((step) => step.id === stepId)
  const step = liveSteps[place]
  return step === undefined
    ? t(($) => $.automations.stepFigures.notLive)
    : `${place + 1}. ${stepLabel(t, step)}`
}
