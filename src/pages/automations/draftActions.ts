import type { TFunction } from 'i18next'

import type { EditorOutcomeTexts } from '../../editing/useEditorEffects.ts'
import {
  siteAutomationsPublishMutation,
  siteAutomationsRevertMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { AutomationOperation } from './editorSession.ts'

// The toasts of an Automation's Save, Publish and Revert, in the editor and on its detail page.
export function automationOutcomeTexts(
  t: TFunction,
): Record<AutomationOperation | 'SAVE', EditorOutcomeTexts> {
  const status = { failedTitle: t(($) => $.alerts.automationStatusErrorTitle) }
  return {
    SAVE: {
      accepted: t(($) => $.notifications.automationUpdated),
      failedTitle: t(($) => $.alerts.automationSaveErrorTitle),
      failedMessage: t(($) => $.alerts.automationSaveErrorMessage),
    },
    PUBLISH: { accepted: t(($) => $.notifications.automationPublished), ...status },
    REVERT: { accepted: t(($) => $.notifications.automationReverted), ...status },
  }
}

// What DraftControls runs for an Automation on its detail page (ADR 0065): its Publish and Revert
// and the messages they show.
export function automationDraftActions(t: TFunction) {
  const texts = automationOutcomeTexts(t)
  return {
    publishMutation: siteAutomationsPublishMutation(),
    revertMutation: siteAutomationsRevertMutation(),
    messages: {
      published: texts.PUBLISH.accepted,
      reverted: texts.REVERT.accepted,
      errorTitle: texts.PUBLISH.failedTitle,
    },
  }
}
