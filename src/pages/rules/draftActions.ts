import type { TFunction } from 'i18next'

import {
  siteRulesPublishMutation,
  siteRulesRevertMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'

// What DraftControls runs for a Rule (ADR 0065): its Publish and Revert and the messages they
// show.
export function ruleDraftActions(t: TFunction) {
  return {
    publishMutation: siteRulesPublishMutation(),
    revertMutation: siteRulesRevertMutation(),
    messages: {
      published: t(($) => $.notifications.rulePublished),
      reverted: t(($) => $.notifications.ruleReverted),
      errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
    },
  }
}
