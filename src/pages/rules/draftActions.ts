import type { TFunction } from 'i18next'

import {
  siteRulesPublishMutation,
  siteRulesRevertMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { ruleOperations } from './ruleEditor.ts'

// What DraftControls runs for a Rule on its detail page (ADR 0065): its Publish and Revert, with
// the messages the editor's operations show.
export function ruleDraftActions(t: TFunction) {
  const published = ruleOperations.PUBLISH.texts(t)
  return {
    publishMutation: siteRulesPublishMutation(),
    revertMutation: siteRulesRevertMutation(),
    messages: {
      published: published.accepted,
      reverted: ruleOperations.REVERT.texts(t).accepted,
      errorTitle: published.failedTitle,
    },
  }
}
