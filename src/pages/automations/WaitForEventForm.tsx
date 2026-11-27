import { Box, Input, NumberInput, Stack, TextInput } from '@mantine/core'
import type { FocusEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { emptyGroup } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { filterText, waitForEventConditionOf } from './definition.ts'
import type { WaitForEventDraft, WaitForEventStep } from './definition.ts'
import { MatcherListControl } from './EndEventControls.tsx'
import { useShownErrors, useValidator } from './stepValidator.ts'
import type { StepValidator } from './stepValidator.ts'
import { useWaitForEventSchema } from './waitForEventRules.ts'

// A Wait for event's panel (ADR 0124): its name, which later Steps read the Event it arrived on by
// and which is fixed once the Step is saved (ADR 0136); how long it waits; the Events it awaits,
// each an Event matcher offered from the built-in events as the Trigger's (ADR 0180); and its
// optional Condition, in the Condition builder of its place (ADR 0181). Its `arrived` and `timedOut`
// paths are the canvas's. Every edit reaches the flow as it is made.
export function WaitForEventForm({
  step,
  storedName,
  checked,
  validatorRef,
  onEdit,
  onChecked,
}: {
  step: WaitForEventStep
  // The name the Step is stored with under its id, which is fixed; undefined before its first Save.
  storedName: string | undefined
  // The Step was checked (on blur or on Save): its errors show as the panel opens.
  checked: boolean
  validatorRef: StepValidator
  onEdit: (draft: WaitForEventDraft) => void
  onChecked: () => void
}) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const schema = useWaitForEventSchema()
  const form = useSchemaForm(schema, step.draft, onEdit)
  useValidator(form, validatorRef)
  useShownErrors(form, checked)
  // A name typed while the first Save was pending is no stored one: it stays editable, so it can be
  // put back.
  const locked = storedName !== undefined && form.values.name === storedName
  // A part edited over several inputs (the Events, the Condition) is checked once focus leaves it.
  const checkOnLeaving = (field: keyof WaitForEventDraft) => (event: FocusEvent) => {
    if (!event.currentTarget.contains(event.relatedTarget)) {
      form.validateFields([field])
    }
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
            initialValue={waitForEventConditionOf(step.draft.conditionText) ?? emptyGroup}
            onChange={(draft) => form.setFieldValue('conditionText', filterText(draft))}
          />
        </Box>
      </Input.Wrapper>
    </Stack>
  )
}
