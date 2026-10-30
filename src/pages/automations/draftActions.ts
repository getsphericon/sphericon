import type { TFunction } from 'i18next'

import {
  siteAutomationsGetQueryKey,
  siteAutomationsListQueryKey,
  siteAutomationsPublishMutation,
  siteAutomationsRevertMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'

// What DraftControls runs for an Automation (ADR 0065): its Publish and Revert, the queries they
// refresh and the messages they show.
export function automationDraftActions(t: TFunction, workspaceId: string, id: string) {
  return {
    publishMutation: siteAutomationsPublishMutation(),
    revertMutation: siteAutomationsRevertMutation(),
    invalidate: [
      siteAutomationsGetQueryKey({ path: { workspaceId, id } }),
      siteAutomationsListQueryKey({ path: { workspaceId } }),
    ],
    messages: {
      published: t(($) => $.notifications.automationPublished),
      reverted: t(($) => $.notifications.automationReverted),
      errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
    },
  }
}
