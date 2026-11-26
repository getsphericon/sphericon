import { ActionIcon, Box, Button, Group, NumberInput, Stack, Text } from '@mantine/core'
import { IconArrowDown, IconArrowUp, IconPlus, IconTrash } from '@tabler/icons-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { emptyGroup } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import type { MessageStep } from '../../components/conditions/ConditionBuilder.tsx'
import { zSplitPathInput } from '../../generated/site/zod.gen.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { boundsOf } from '../../utils/bounds.ts'
import { allSteps, branchConditionOf, filterText, sendsMessage, stepsBefore } from './definition.ts'
import type { BranchStep, FlowPath, FlowStep, ForkStep, SplitStep } from './definition.ts'
import type { PathPatch } from './flowEdits.ts'
import { canAddPath, canRemovePath, forkError, forkSchemas, weightSum } from './forkRules.ts'
import { useStepInfo } from './stepKinds.ts'
import { useShownErrors, useValidator } from './stepValidator.ts'
import type { StepValidator } from './stepValidator.ts'

// The range of a Split path's weight, as the contract bounds it.
const WEIGHT = boundsOf(zSplitPathInput.shape.weight)

// What a fork's panel does to the flow: it sets its paths' Conditions or weights by path id, and
// adds, removes and (on a Branch) moves a path. The Steps the paths hold are the canvas's.
export interface ForkFormProps<S extends ForkStep> {
  step: S
  // The fork was checked (on blur or on Save): its errors show as the panel opens.
  checked: boolean
  validatorRef: StepValidator
  onPaths: (patches: PathPatch[]) => void
  onChecked: () => void
  onAddPath: () => void
  onRemovePath: (pathId: string) => void
  onMovePath: (pathId: string, by: -1 | 1) => void
}

// The buttons beside a path: move it up or down the order (a Branch's), and remove it.
function PathActions({
  canRemove,
  onRemove,
  onMove,
  first,
  last,
}: {
  canRemove: boolean
  onRemove: () => void
  onMove?: (by: -1 | 1) => void
  first: boolean
  last: boolean
}) {
  const { t } = useTranslation()
  return (
    <Group gap={4} wrap="nowrap">
      {onMove && (
        <>
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t(($) => $.automations.movePathUp)}
            disabled={first}
            onClick={() => onMove(-1)}
          >
            <IconArrowUp size={16} />
          </ActionIcon>
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={t(($) => $.automations.movePathDown)}
            disabled={last}
            onClick={() => onMove(1)}
          >
            <IconArrowDown size={16} />
          </ActionIcon>
        </>
      )}
      <ActionIcon
        variant="subtle"
        color="gray"
        aria-label={t(($) => $.automations.removePath)}
        disabled={!canRemove}
        onClick={onRemove}
      >
        <IconTrash size={16} />
      </ActionIcon>
    </Group>
  )
}

function AddPathButton({ step, onAddPath }: { step: ForkStep; onAddPath: () => void }) {
  const { t } = useTranslation()
  return (
    <Group>
      <Button
        variant="light"
        size="compact-sm"
        leftSection={<IconPlus size={14} />}
        disabled={!canAddPath(step)}
        onClick={onAddPath}
      >
        {t(($) => $.automations.addPath)}
      </Button>
    </Group>
  )
}

// The send Steps whose message the Branch may test (ADR 0122): those an Enrollment may have passed
// when it reaches it, each named by its place on the canvas, its kind (infoOf) and an email's
// subject.
function messageStepsOf(
  infoOf: (step: FlowStep) => { label: string },
  root: FlowPath,
  branch: BranchStep,
): MessageStep[] {
  const places = allSteps(root).map((step) => step.id)
  return stepsBefore(root, branch.id)
    .filter((step) => sendsMessage(step))
    .map((step) => {
      const kind = infoOf(step).label
      const named =
        step.kind === 'email' && step.draft.subject !== '' ? `${kind}: ${step.draft.subject}` : kind
      return {
        id: step.id,
        label: `${places.indexOf(step.id) + 1}. ${named}`,
        opens: step.kind === 'email',
      }
    })
}

// A Branch's panel: each listed path's Condition, in the Condition builder of the Branch path's
// place (ADR 0181), which tests the Contact and the Event the Enrollment began with, in the order
// they are tested, and the messages of the send Steps before it (root is the version's Steps); then
// `otherwise` and, while a Condition can be unknown, the unknown path.
export function BranchForm({
  root,
  step,
  checked,
  validatorRef,
  onPaths,
  onChecked,
  onAddPath,
  onRemovePath,
  onMovePath,
}: ForkFormProps<BranchStep> & { root: FlowPath }) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const infoOf = useStepInfo()
  const messageSteps = useMemo(() => messageStepsOf(infoOf, root, step), [infoOf, root, step])
  const form = useSchemaForm(
    forkSchemas.branch,
    { paths: step.paths.map(({ id, conditionText }) => ({ id, conditionText })) },
    (values) => onPaths(values.paths),
  )
  const fields = step.paths.map((_, i) => `paths.${i}.conditionText`)
  useValidator(form, validatorRef, fields)
  useShownErrors(form, checked, fields)

  return (
    <Stack>
      {step.paths.map((path, i) => {
        const field = `paths.${i}.conditionText`
        const error = form.errors[field]
        return (
          <Stack key={path.id} gap="xs">
            <Group justify="space-between" wrap="nowrap">
              <Text fw={500}>{t(($) => $.automations.pathNumbered, { number: i + 1 })}</Text>
              <PathActions
                canRemove={canRemovePath(step)}
                onRemove={() => onRemovePath(path.id)}
                onMove={(by) => onMovePath(path.id, by)}
                first={i === 0}
                last={i === step.paths.length - 1}
              />
            </Group>
            {/* The Condition is checked once focus leaves its builder, not on each of its
                inputs: a test is built over several of them. */}
            <Box
              onBlur={(event) => {
                if (!event.currentTarget.contains(event.relatedTarget)) {
                  form.validateFields([field])
                  onChecked()
                }
              }}
            >
              <ConditionBuilder
                workspaceId={workspaceId}
                place="branch"
                messageSteps={messageSteps}
                initialValue={branchConditionOf(path.conditionText) ?? emptyGroup}
                onChange={(draft) => form.setFieldValue(field, filterText(draft))}
              />
            </Box>
            {error && (
              <Text size="sm" c="red">
                {error}
              </Text>
            )}
          </Stack>
        )
      })}
      <AddPathButton step={step} onAddPath={onAddPath} />
      <Text size="sm" c="dimmed">
        {t(($) => $.automations.pathOtherwiseHint)}
      </Text>
      {step.unknown && (
        <Text size="sm" c="dimmed">
          {t(($) => $.automations.pathUnknownHint)}
        </Text>
      )}
    </Stack>
  )
}

// A Split's panel: each path's weight, a whole percentage, with their sum, which must be 100. The
// first path is the one the others are compared with (ADR 0192), so the paths keep their order.
export function SplitForm({
  step,
  checked,
  validatorRef,
  onPaths,
  onChecked,
  onAddPath,
  onRemovePath,
}: ForkFormProps<SplitStep>) {
  const { t } = useTranslation()
  const form = useSchemaForm(
    forkSchemas.split,
    { paths: step.paths.map(({ id, weight }) => ({ id, weight })) },
    (values) => onPaths(values.paths),
  )
  const fields = step.paths.map((_, i) => `paths.${i}.weight`)
  useValidator(form, validatorRef, fields)
  useShownErrors(form, checked, fields)
  const error = forkError(step)
  const sum = weightSum(step.paths)

  return (
    <Stack onBlur={onChecked}>
      {step.paths.map((path, i) => (
        <Group key={path.id} align="flex-end" wrap="nowrap">
          <NumberInput
            flex={1}
            label={t(($) => $.automations.pathWeightLabel, { number: i + 1 })}
            suffix="%"
            min={WEIGHT.min}
            allowDecimal={false}
            allowNegative={false}
            {...form.getInputProps(`paths.${i}.weight`)}
          />
          <PathActions
            canRemove={canRemovePath(step)}
            onRemove={() => onRemovePath(path.id)}
            first={i === 0}
            last={i === step.paths.length - 1}
          />
        </Group>
      ))}
      <Text size="sm" {...(error?.kind === 'sum' && { c: 'red' })}>
        {t(($) => $.automations.weightSum, { sum })}
      </Text>
      {checked && error?.kind === 'sum' && (
        <Text size="sm" c="red">
          {t(($) => $.automations.weightSumError, { sum })}
        </Text>
      )}
      <AddPathButton step={step} onAddPath={onAddPath} />
    </Stack>
  )
}
