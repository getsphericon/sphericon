import {
  ActionIcon,
  CloseButton,
  Fieldset,
  Group,
  Paper,
  ScrollArea,
  Text,
  Title,
} from '@mantine/core'
import { IconCopy } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { allSteps, forkPaths } from './definition.ts'
import type { EditedStep, Flow, FlowStep, TriggerDraft } from './definition.ts'
import type { PathPatch } from './flowEdits.ts'
import { TRIGGER_NODE_ID } from './flowLayout.ts'
import { BranchForm, SplitForm } from './ForkForms.tsx'
import { StepForm } from './StepForms.tsx'
import { useStepInfo } from './stepKinds.ts'
import type { StepValidator } from './stepValidator.ts'
import { TriggerPanel } from './TriggerPanel.tsx'

// What the panel does to a fork of the id (ForkForms.tsx), and whether the fork was checked.
export interface ForkControls {
  checked: (id: string) => boolean
  onChecked: (id: string) => void
  onPaths: (id: string, patches: PathPatch[]) => void
  onAddPath: (id: string) => void
  onRemovePath: (id: string, pathId: string) => void
  onMovePath: (id: string, move: { pathId: string; by: -1 | 1 }) => void
}

// The form of the selected Step: its kind's own, a fork's paths, or what an End does.
function StepBody({
  step,
  validatorRef,
  onStep,
  forks,
}: {
  step: FlowStep
  validatorRef: StepValidator
  onStep: (step: EditedStep) => void
  forks: ForkControls
}) {
  const { t } = useTranslation()
  // A fork's form opens anew when its paths change (added, removed, moved): it holds one field
  // per path, in their order.
  const key = forkPaths(step)
    .map((path) => path.id)
    .join(':')
  const forkProps = {
    checked: forks.checked(step.id),
    validatorRef,
    onPaths: (patches: PathPatch[]) => forks.onPaths(step.id, patches),
    onChecked: () => forks.onChecked(step.id),
    onAddPath: () => forks.onAddPath(step.id),
    onRemovePath: (pathId: string) => forks.onRemovePath(step.id, pathId),
    onMovePath: (pathId: string, by: -1 | 1) => forks.onMovePath(step.id, { pathId, by }),
  }
  if (step.kind === 'kept') {
    return null
  }
  if (step.kind === 'end') {
    return <Text size="sm">{t(($) => $.automations.endNodeDescription)}</Text>
  }
  if (step.kind === 'branch') {
    return <BranchForm key={key} step={step} {...forkProps} />
  }
  if (step.kind === 'split') {
    return <SplitForm key={key} step={step} {...forkProps} />
  }
  return <StepForm step={step} onEdit={onStep} validatorRef={validatorRef} />
}

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
  forks,
  onCopy,
  onClose,
}: {
  flow: Flow
  selected: string
  editable: boolean
  validatorRef: StepValidator
  onTrigger: (patch: Partial<TriggerDraft>) => void
  onStep: (step: EditedStep) => void
  forks: ForkControls
  onCopy: (id: string) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const info = useStepInfo()
  const step = allSteps(flow.path).find((candidate) => candidate.id === selected)
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
            {/* An End is its path's last Step: a copy after it would never run. */}
            {editable && step !== undefined && step.kind !== 'kept' && step.kind !== 'end' && (
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
            <StepBody step={step} validatorRef={validatorRef} onStep={onStep} forks={forks} />
          )}
        </Fieldset>
      </ScrollArea>
    </Paper>
  )
}
