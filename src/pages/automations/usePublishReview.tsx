import { Modal } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { AutomationStepWarnings } from '../../generated/site/types.gen.ts'
import type { Flow } from './definition.ts'
import { PublishReview } from './PublishReview.tsx'

// usePublishReview is the last look before Publish arms the saved version: the lint's warnings of
// its email steps (ADR 0188), each step's under its name, over the flow of that version. They hold
// nothing back: `review` runs the Publish it is given at once while there is no warning, and once
// the person confirms otherwise. `modal` is placed beside the Publish button.
export function usePublishReview(flow: Flow, warnings: AutomationStepWarnings[] | undefined) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<(() => void) | null>(null)
  const review = (publish: () => void) => {
    if (warnings === undefined || warnings.length === 0) {
      publish()
      return
    }
    setPending(() => publish)
  }
  const modal = (
    <Modal
      opened={pending !== null}
      onClose={() => setPending(null)}
      title={t(($) => $.automations.publishReview.title)}
    >
      {pending !== null && (
        <PublishReview
          flow={flow}
          warnings={warnings ?? []}
          onCancel={() => setPending(null)}
          onConfirm={() => {
            setPending(null)
            pending()
          }}
        />
      )}
    </Modal>
  )
  return { review, modal }
}
