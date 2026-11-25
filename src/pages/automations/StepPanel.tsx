import { ActionIcon, CloseButton, Fieldset, Group, Paper, ScrollArea, Title } from '@mantine/core'
import { IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { EditedStep, Flow, TriggerDraft } from './definition.ts'
import { TRIGGER_NODE_ID } from './flowLayout.ts'
import { StepForm } from './StepForms.tsx'
import type { StepValidator } from './StepForms.tsx'
import { useStepInfo } from './stepKinds.ts'
import { TriggerPanel } from './TriggerPanel.tsx'

// The side panel edits what is selected on the canvas: the Trigger, with its exit and goal events
// and its entry settings, or a Step, in its kind's form. Every edit reaches the flow as it is made.
// A version the editor cannot change, or one an operation is acting on, shows it disabled.
export function StepPanel({
  flow,
  selected,
  editable,
  validatorRef,
  onTrigger,
  onStep,
  onCopy,
  onClose,
}: {
  flow: Flow
  selected: string
  editable: boolean
  validatorRef: StepValidator
  onTrigger: (patch: Partial<TriggerDraft>) => void
  onStep: (step: EditedStep) => void
  onCopy: (id: string) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const info = useStepInfo()
  const step = flow.path.steps.find((candidate) => candidate.id === selected)
  if (selected !== TRIGGER_NODE_ID && step === undefined) {
    return null
  }
  const title = step === undefined ? t(($) => $.automations.stepTrigger) : info(step).label

  return (
    <Paper
      withBorder
      shadow="sm"
      pos="absolute"
      top={88}
      right={16}
      bottom={16}
      w={{ base: 'calc(100% - 32px)', sm: 420 }}
      p="md"
      component="section"
      aria-label={title}
    >
      <ScrollArea h="100%">
        <Group justify="space-between" wrap="nowrap" mb="md">
          <Title order={4}>{title}</Title>
          <Group gap="xs" wrap="nowrap">
            {editable && step !== undefined && step.kind !== 'kept' && (
              <ActionIcon
                variant="subtle"
                color="gray"
                aria-label={t(($) => $.automations.copyStep)}
                onClick={() => onCopy(step.id)}
              >
                <IconCopy size={16} />
              </ActionIcon>
            )}
            <CloseButton aria-label={t(($) => $.automations.closePanel)} onClick={onClose} />
          </Group>
        </Group>
        <Fieldset disabled={!editable} variant="unstyled" m={0} miw={0}>
          {step === undefined ? (
            <TriggerPanel trigger={flow.trigger} onEdit={onTrigger} />
          ) : (
            step.kind !== 'kept' && (
              <StepForm step={step} onEdit={onStep} validatorRef={validatorRef} />
            )
          )}
        </Fieldset>
      </ScrollArea>
    </Paper>
  )
}
