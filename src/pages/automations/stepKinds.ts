import {
  IconArrowsSplit,
  IconBolt,
  IconBrandTelegram,
  IconCalendarTime,
  IconClock,
  IconFlag,
  IconGitBranch,
  IconMail,
  IconMessage,
  IconPuzzle,
  IconTag,
  IconTagOff,
  IconWebhook,
} from '@tabler/icons-react'
import type { Icon } from '@tabler/icons-react'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import type { FlowStep, InsertedKind } from './definition.ts'

export interface StepKindInfo {
  label: string
  description: string
  icon: Icon
}

// useStepKinds names each kind a person puts on the canvas, says what it does and gives it an icon,
// in the order the insertion menu offers them. It is the same object until the language changes.
export function useStepKinds(): Record<InsertedKind, StepKindInfo> {
  const { t } = useTranslation()
  return useMemo(
    () => ({
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
      wait_until: {
        label: t(($) => $.automations.stepWaitUntil),
        description: t(($) => $.automations.waitUntilNodeDescription),
        icon: IconCalendarTime,
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
      branch: {
        label: t(($) => $.automations.stepBranch),
        description: t(($) => $.automations.branchNodeDescription),
        icon: IconGitBranch,
      },
      split: {
        label: t(($) => $.automations.stepSplit),
        description: t(($) => $.automations.splitNodeDescription),
        icon: IconArrowsSplit,
      },
      end: {
        label: t(($) => $.automations.stepEnd),
        description: t(($) => $.automations.endNodeDescription),
        icon: IconFlag,
      },
    }),
    [t],
  )
}

// The kinds in the order the insertion menu offers them. An End finishes its path, so it is
// offered only at a path's end.
export function insertableKinds(kinds: Record<InsertedKind, StepKindInfo>, pathEnd: boolean) {
  return Object.keys(kinds).flatMap((kind) =>
    isKind(kinds, kind) && (pathEnd || kind !== 'end') ? [{ kind, ...kinds[kind] }] : [],
  )
}

function isKind(kinds: Record<InsertedKind, StepKindInfo>, kind: string): kind is InsertedKind {
  return Object.hasOwn(kinds, kind)
}

export const TRIGGER_ICON = IconBolt

// useStepInfo is what a node shows of its Step: its kind's name and icon. A kind newer than the
// editor is named by its type as stored. It is the same function until the language changes.
export function useStepInfo(): (step: FlowStep) => { label: string; icon: Icon } {
  const kinds = useStepKinds()
  return useCallback(
    (step: FlowStep) =>
      step.kind === 'kept' ? { label: step.step.type, icon: IconPuzzle } : kinds[step.kind],
    [kinds],
  )
}
