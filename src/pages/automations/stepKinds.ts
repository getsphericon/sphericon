import {
  IconBolt,
  IconBrandTelegram,
  IconClock,
  IconFlag,
  IconMail,
  IconMessage,
  IconPuzzle,
  IconTag,
  IconTagOff,
  IconWebhook,
} from '@tabler/icons-react'
import type { Icon } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { EditedKind, FlowStep } from './definition.ts'

export interface StepKindInfo {
  label: string
  description: string
  icon: Icon
}

// useStepKinds names each kind the editor edits, says what it does and gives it an icon, in the
// order the insertion menu offers them.
export function useStepKinds(): Record<EditedKind, StepKindInfo> {
  const { t } = useTranslation()
  return {
    email: {
      label: t(($) => $.automations.stepEmail),
      description: t(($) => $.automations.emailNodeDescription),
      icon: IconMail,
    },
    telegram: {
      label: t(($) => $.automations.stepTelegram),
      description: t(($) => $.automations.telegramNodeDescription),
      icon: IconBrandTelegram,
    },
    sms: {
      label: t(($) => $.automations.stepSms),
      description: t(($) => $.automations.smsNodeDescription),
      icon: IconMessage,
    },
    wait: {
      label: t(($) => $.automations.stepWait),
      description: t(($) => $.automations.waitNodeDescription),
      icon: IconClock,
    },
    apply_tag: {
      label: t(($) => $.automations.stepApplyTag),
      description: t(($) => $.automations.applyTagNodeDescription),
      icon: IconTag,
    },
    remove_tag: {
      label: t(($) => $.automations.stepRemoveTag),
      description: t(($) => $.automations.removeTagNodeDescription),
      icon: IconTagOff,
    },
    http: {
      label: t(($) => $.automations.stepHttp),
      description: t(($) => $.automations.httpNodeDescription),
      icon: IconWebhook,
    },
  }
}

// The kinds in the order the insertion menu offers them.
export function insertableKinds(kinds: Record<EditedKind, StepKindInfo>) {
  return Object.keys(kinds).flatMap((kind) =>
    isKind(kinds, kind) ? [{ kind, ...kinds[kind] }] : [],
  )
}

function isKind(kinds: Record<EditedKind, StepKindInfo>, kind: string): kind is EditedKind {
  return Object.hasOwn(kinds, kind)
}

export const TRIGGER_ICON = IconBolt

// useStepInfo is what a node shows of its Step: its kind's name and icon. A Step the editor does
// not edit is named by its kind as stored: an End by its name, a kind newer than the editor by its
// type.
export function useStepInfo(): (step: FlowStep) => { label: string; icon: Icon } {
  const { t } = useTranslation()
  const kinds = useStepKinds()
  return (step) => {
    if (step.kind !== 'kept') {
      return kinds[step.kind]
    }
    const { type } = step.step
    return type === 'end'
      ? { label: t(($) => $.automations.stepEnd), icon: IconFlag }
      : { label: type, icon: IconPuzzle }
  }
}
