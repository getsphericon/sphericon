import { Box, Input, NumberInput, SegmentedControl, Select, Stack, TextInput } from '@mantine/core'
import type { FocusEvent } from 'react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { emptyGroup } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import type { MessageStep } from '../../components/conditions/ConditionBuilder.tsx'
import { zMessageEngagement } from '../../generated/site/zod.gen.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import type { SchemaForm } from '../../resources/schemaForm.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { AWAITED_FORMS, filterText, waitForEventConditionOf } from './definition.ts'
import type { AwaitedForm, FlowPath, WaitForEventDraft, WaitForEventStep } from './definition.ts'
import { MatcherListControl } from './EndEventControls.tsx'
import { messageStepsOf } from './messageSteps.ts'
import { useStepInfo } from './stepKinds.ts'
import { useShownErrors, useValidator } from './stepValidator.ts'
import type { StepValidator } from './stepValidator.ts'
import { useWaitForEventSchema } from './waitForEventRules.ts'

type WaitForEventForm = SchemaForm<WaitForEventDraft, unknown>

// The Event form's fields: the Events awaited, each an Event matcher offered from the built-in
// events as the Trigger's (ADR 0180), and the optional Condition, in the Condition builder of its
// place (ADR 0181).
function AwaitedEventFields({ form, draft }: { form: WaitForEventForm; draft: WaitForEventDraft }) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  // A part edited over several inputs (the Events, the Condition) is checked once focus leaves it.
  const checkOnLeaving = (field: keyof WaitForEventDraft) => (event: FocusEvent) => {
    if (!event.currentTarget.contains(event.relatedTarget)) {
      form.validateFields([field])
    }
  }
  return (
    <>
      <Box onBlur={checkOnLeaving('matchers')}>
        <MatcherListControl
          label={t(($) => $.automations.waitForEvent.matchersLabel)}
          description={t(($) => $.automations.waitForEvent.matchersHint)}
          addLabel={t(($) => $.automations.waitForEvent.matcherAdd)}
          error={form.errors.matchers}
          value={form.values.matchers}
          onChange={(matchers) => form.setFieldValue('matchers', matchers)}
        />
      </Box>
      <Input.Wrapper
        label={t(($) => $.automations.waitForEvent.conditionLabel)}
        description={t(($) => $.automations.waitForEvent.conditionHint)}
        error={form.errors.conditionText}
      >
        <Box mt="xs" onBlur={checkOnLeaving('conditionText')}>
          <ConditionBuilder
            workspaceId={workspaceId}
            place="waitForEvent"
            initialValue={waitForEventConditionOf(draft.conditionText) ?? emptyGroup}
            onChange={(condition) => form.setFieldValue('conditionText', filterText(condition))}
          />
        </Box>
      </Input.Wrapper>
    </>
  )
}

// The open or click form's fields: the send Step whose message it awaits, picked from those that
// can come before the wait (ADR 0122), and whether it awaits an open, which only an email's message
// is tracked for, or a click.
function AwaitedMessageFields({
  form,
  messageSteps,
}: {
  form: WaitForEventForm
  messageSteps: MessageStep[]
}) {
  const { t } = useTranslation()
  const engagements = {
    opened: t(($) => $.automations.waitForEvent.engagements.opened),
    clicked: t(($) => $.automations.waitForEvent.engagements.clicked),
  }
  const picked = messageSteps.find((step) => step.id === form.values.messageStep)
  const stepProps = form.getInputProps('messageStep')
  return (
    <>
      <Select
        label={t(($) => $.automations.waitForEvent.messageStepLabel)}
        description={t(($) => $.automations.waitForEvent.messageStepHint)}
        data={messageSteps.map((step) => ({ value: step.id, label: step.label }))}
        allowDeselect={false}
        {...stepProps}
        value={picked === undefined ? null : picked.id}
        onChange={(id) => {
          const step = messageSteps.find((candidate) => candidate.id === id)
          stepProps.onChange(step?.id ?? '')
          // Only an email's opens are tracked: another Channel's message is awaited clicked.
          if (step !== undefined && !step.opens) {
            form.setFieldValue('engagement', 'clicked')
          }
        }}
      />
      <Input.Wrapper
        label={t(($) => $.automations.waitForEvent.engagementLabel)}
        error={form.errors.engagement}
      >
        <SegmentedControl
          display="flex"
          data={zMessageEngagement.options.map((engagement) => ({
            value: engagement,
            label: engagements[engagement],
            disabled: engagement === 'opened' && picked !== undefined && !picked.opens,
          }))}
          value={form.values.engagement}
          onChange={(value) => {
            const parsed = zMessageEngagement.safeParse(value)
            if (parsed.success) {
              form.setFieldValue('engagement', parsed.data)
            }
          }}
        />
      </Input.Wrapper>
    </>
  )
}

// A Wait for event's panel (ADR 0124): its name, which later Steps read the Event it arrived on by
// and which is fixed once the Step is saved (ADR 0136); how long it waits; and what it awaits, in
// one of two forms: Events with an optional Condition, or an open or click of the message a send
// Step before it sent (root is the version's Steps). Switching the form is the flow's edit
// (onForm): it adds or removes the unknown path, which may ask first. Its paths are the canvas's.
// Every other edit reaches the flow as it is made.
export function WaitForEventForm({
  root,
  step,
  storedName,
  checked,
  validatorRef,
  onEdit,
  onForm,
  onChecked,
}: {
  root: FlowPath
  step: WaitForEventStep
  // The name the Step is stored with under its id, which is fixed; undefined before its first Save.
  storedName: string | undefined
  // The Step was checked (on blur or on Save): its errors show as the panel opens.
  checked: boolean
  validatorRef: StepValidator
  onEdit: (draft: WaitForEventDraft) => void
  onForm: (form: AwaitedForm) => void
  onChecked: () => void
}) {
  const { t } = useTranslation()
  const infoOf = useStepInfo()
  const messageSteps = useMemo(() => messageStepsOf(infoOf, root, step.id), [infoOf, root, step.id])
  const schemaOf = useWaitForEventSchema()
  const schema = useMemo(() => schemaOf(messageSteps), [schemaOf, messageSteps])
  const form = useSchemaForm(schema, step.draft, onEdit)
  useValidator(form, validatorRef)
  useShownErrors(form, checked)
  // A name typed while the first Save was pending is no stored one: it stays editable, so it can be
  // put back.
  const locked = storedName !== undefined && form.values.name === storedName
  const forms: Record<AwaitedForm, string> = {
    event: t(($) => $.automations.waitForEvent.forms.event),
    message: t(($) => $.automations.waitForEvent.forms.message),
  }

  return (
    <Stack onBlur={onChecked}>
      <TextInput
        label={t(($) => $.automations.waitForEvent.nameLabel)}
        description={
          locked
            ? t(($) => $.automations.waitForEvent.nameLocked)
            : t(($) => $.automations.waitForEvent.nameHint)
        }
        disabled={locked}
        {...form.getInputProps('name')}
      />
      <NumberInput
        label={t(($) => $.automations.waitForEvent.timeoutLabel)}
        description={t(($) => $.automations.waitForEvent.timeoutHint)}
        min={1}
        allowDecimal={false}
        allowNegative={false}
        {...form.getInputProps('timeoutSeconds')}
      />
      <Input.Wrapper label={t(($) => $.automations.waitForEvent.formLabel)}>
        <SegmentedControl
          display="flex"
          data={AWAITED_FORMS.map((awaited) => ({ value: awaited, label: forms[awaited] }))}
          // The flow's, not the form's: a removal of the unknown path that is not confirmed leaves
          // the form as it was.
          value={step.draft.form}
          onChange={(picked) => {
            const awaited = AWAITED_FORMS.find((candidate) => candidate === picked)
            if (awaited !== undefined) {
              onForm(awaited)
            }
          }}
        />
      </Input.Wrapper>
      {step.draft.form === 'message' ? (
        <AwaitedMessageFields form={form} messageSteps={messageSteps} />
      ) : (
        <AwaitedEventFields form={form} draft={step.draft} />
      )}
    </Stack>
  )
}
