import type { TFunction } from 'i18next'

import type { HoldReason } from '../generated/site/types.gen.ts'

// holdExplanation is what lifts a hold of the given reason (Outbound send, ADR 0015), in words: a
// held Broadcast and an Automation that cannot send say the same.
export function holdExplanation(t: TFunction, reason: HoldReason): string {
  return t(($) => $.holdReasons[reason])
}
