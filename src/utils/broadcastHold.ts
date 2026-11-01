import type { TFunction } from 'i18next'

import type { BroadcastHoldReason } from '../generated/site/types.gen.ts'

// holdExplanation is what lifts a hold of the given reason (Outbound send, ADR 0015), in words.
export function holdExplanation(t: TFunction, reason: BroadcastHoldReason): string {
  return t(($) => $.broadcasts.hold.reasons[reason])
}
