import { Button, Group, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { LintWarnings } from '../../components/LintWarnings.tsx'
import type { AutomationStepWarnings } from '../../generated/site/types.gen.ts'
import { allSteps } from './definition.ts'
import type { Flow } from './definition.ts'
import { useStepInfo } from './stepKinds.ts'

// PublishReview lists the warnings of the email steps Publish arms, each step's under its name,
// with Publish and Cancel.
export function PublishReview({
  flow,
  warnings,
  onCancel,
  onConfirm,
}: {
  flow: Flow
  warnings: AutomationStepWarnings[]
  onCancel: () => void
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  const info = useStepInfo()
  const steps = allSteps(flow.path)
  // A step is named by its kind and, for an email, its subject, in the order of the canvas.
  const named = warnings
    .map((entry) => {
      const step = steps.find((candidate) => candidate.id === entry.stepId)
      const label =
        step?.kind === 'email' && step.draft.subject !== ''
          ? `${info(step).label}: ${step.draft.subject}`
          : t(($) => $.automations.stepEmail)
      return { ...entry, label, place: step === undefined ? steps.length : steps.indexOf(step) }
    })
    .toSorted((a, b) => a.place - b.place)

  return (
    <Stack>
      <Text size="sm">{t(($) => $.automations.publishReview.description)}</Text>
      {named.map((entry) => (
        <LintWarnings key={entry.stepId} title={entry.label} warnings={entry.warnings} />
      ))}
      <Group justify="flex-end">
        <Button variant="default" onClick={onCancel}>
          {t(($) => $.actions.cancel)}
        </Button>
        <Button onClick={onConfirm}>{t(($) => $.draft.publish)}</Button>
      </Group>
    </Stack>
  )
}
