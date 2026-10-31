import type { TFunction } from 'i18next'

import {
  siteAutomationsPublishMutation,
  siteAutomationsRevertMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'

// What DraftControls runs for an Automation (ADR 0065): its Publish and Revert and the messages
// they show.
export function automationDraftActions(t: TFunction) {
  return {
    publishMutation: siteAutomationsPublishMutation(),
    revertMutation: siteAutomationsRevertMutation(),
    messages: {
      published: t(($) => $.notifications.automationPublished),
      reverted: t(($) => $.notifications.automationReverted),
      errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
    },
  }
}
