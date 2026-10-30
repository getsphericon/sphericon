import type { TFunction } from 'i18next'

import {
  siteRulesGetQueryKey,
  siteRulesListQueryKey,
  siteRulesPublishMutation,
  siteRulesRevertMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'

// What DraftControls runs for a Rule (ADR 0065): its Publish and Revert, the queries they refresh
// and the messages they show.
export function ruleDraftActions(t: TFunction, workspaceId: string, id: string) {
  return {
    publishMutation: siteRulesPublishMutation(),
    revertMutation: siteRulesRevertMutation(),
    invalidate: [
      siteRulesGetQueryKey({ path: { workspaceId, id } }),
      siteRulesListQueryKey({ path: { workspaceId } }),
    ],
    messages: {
      published: t(($) => $.notifications.rulePublished),
      reverted: t(($) => $.notifications.ruleReverted),
      errorTitle: t(($) => $.alerts.ruleStatusErrorTitle),
    },
  }
}
