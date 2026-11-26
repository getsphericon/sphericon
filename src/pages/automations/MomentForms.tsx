import { Chip, Group, Input, NumberInput, SegmentedControl, Select, Stack } from '@mantine/core'
import { TimeInput } from '@mantine/dates'
import type { TimeInputProps } from '@mantine/dates'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteWorkspacesListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  DateFieldInput,
  DateRecurrence,
  SiteCustomFieldResource,
  Weekday,
} from '../../generated/site/types.gen.ts'
import { zDateFieldInput, zDateRecurrence, zWeekday } from '../../generated/site/zod.gen.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { jsonText } from '../../utils/jsonText.ts'
import type { DateDraft, OffsetSign, TimeOfDayDraft } from './definition.ts'
import {
  offsetDaysBound,
  timeOfDaySchema,
  useDateSchema,
  useDatetimeFields,
} from './momentRules.ts'
import { useShownErrors, useValidator } from './stepValidator.ts'
import type { StepValidator } from './stepValidator.ts'

// The forms of the moments a date Trigger and a Wait until name (ADR 0126). The date form is one
// component, which the Trigger node and a Wait until on a date both show (story 63 of #1613).

// What a moment's form is given: the draft it opens with, where every edit goes, the check the
// editor's Save runs, and whether its node was checked, which shows its errors as it opens.
export interface MomentFormProps<T> {
  value: T
  onChange: (value: T) => void
  validatorRef: StepValidator
  checked: boolean
  // Focus left the form: its node shows its errors from then on.
  onChecked: () => void
}

// The days of the week in the contract's order, Monday first.
const WEEKDAYS = zWeekday.options

// The sides of the date an offset counts on: before it, then after it.
const SIGNS: OffsetSign[] = [-1, 1]

// A date field as the picker holds it: its reference as text, which keeps a Custom field's key
// apart from the birthday whatever the key.
function pickerValue(field: DateFieldInput): string {
  return JSON.stringify(field)
}

function pickedField(value: string | null): DateFieldInput | null {
  if (value === null) {
    return null
  }
  const parsed = jsonText.pipe(zDateFieldInput).safeParse(value)
  return parsed.success ? parsed.data : null
}

// The Workspace time zone (ADR 0057), which a Contact without a valid time zone of its own is read
// in; undefined until the Workspace is read.
function useWorkspaceZone(): string | undefined {
  const { workspaceId } = workspaceRoute.useParams()
  const workspaces = useQuery(siteWorkspacesListOptions())
  return workspaces.data?.find((w) => w.id === workspaceId)?.timezone
}

// The local time of a moment, in whole minutes, read in each Contact's time zone; the note under it
// names the Workspace time zone a Contact without one is read in.
function LocalTimeInput(inputProps: TimeInputProps) {
  const { t } = useTranslation()
  const zone = useWorkspaceZone()
  return (
    <TimeInput
      label={t(($) => $.automations.localTimeLabel)}
      {...(zone !== undefined && {
        description: t(($) => $.automations.localTimeHint, { zone }),
      })}
      {...inputProps}
    />
  )
}

// The field picker of a date: the birthday every Contact has, then the catalogue's datetime Custom
// fields. A field the version names that the catalogue no longer holds as a datetime one is listed
// by its key, so its error shows beside it.
function fieldOptions(
  t: ReturnType<typeof useTranslation>['t'],
  datetimeFields: SiteCustomFieldResource[] | undefined,
  current: DateFieldInput | null,
) {
  const custom = (datetimeFields ?? []).map((f) => ({
    value: pickerValue({ kind: 'customField', key: f.key }),
    label: f.name,
  }))
  const stray =
    current?.kind === 'customField' &&
    !custom.some((option) => option.value === pickerValue(current))
      ? [{ value: pickerValue(current), label: current.key }]
      : []
  return [
    {
      group: t(($) => $.automations.date.fieldGroupContact),
      items: [
        { value: pickerValue({ kind: 'birthday' }), label: t(($) => $.automations.date.birthday) },
      ],
    },
    { group: t(($) => $.automations.date.fieldGroupCustom), items: [...custom, ...stray] },
  ]
}

// DateOccurrenceForm edits a date the Contact holds (ADR 0126): its field, how often it comes,
// how many days before or after it, and at what local time.
export function DateOccurrenceForm({
  value,
  onChange,
  validatorRef,
  checked,
  onChecked,
}: MomentFormProps<DateDraft>) {
  const { t } = useTranslation()
  const datetimeFields = useDatetimeFields()
  const schema = useDateSchema(datetimeFields)
  const form = useSchemaForm(schema, value, onChange)
  useValidator(form, validatorRef)
  useShownErrors(form, checked)
  const { values } = form
  const birthday = values.field?.kind === 'birthday'
  const recurrences: Record<DateRecurrence, string> = {
    yearly: t(($) => $.automations.date.recurrence.yearly),
    once: t(($) => $.automations.date.recurrence.once),
  }
  const sides = {
    before: t(($) => $.automations.date.direction.before),
    after: t(($) => $.automations.date.direction.after),
  }
  const signProps = form.getInputProps('sign')
  const fieldProps = form.getInputProps('field')

  return (
    <Stack onBlur={onChecked}>
      <Select
        label={t(($) => $.automations.date.fieldLabel)}
        data={fieldOptions(t, datetimeFields, values.field)}
        allowDeselect={false}
        {...fieldProps}
        value={values.field === null ? null : pickerValue(values.field)}
        onChange={(picked) => {
          const field = pickedField(picked)
          fieldProps.onChange(field)
          // The birthday is always past, so it only comes yearly.
          if (field?.kind === 'birthday') {
            form.setFieldValue('recurrence', zDateRecurrence.enum.yearly)
          }
        }}
      />
      <Input.Wrapper
        label={t(($) => $.automations.date.recurrenceLabel)}
        error={form.errors.recurrence}
      >
        <SegmentedControl
          display="flex"
          data={zDateRecurrence.options.map((recurrence) => ({
            value: recurrence,
            label: recurrences[recurrence],
            disabled: birthday && recurrence === zDateRecurrence.enum.once,
          }))}
          value={values.recurrence}
          onChange={(picked) => {
            const parsed = zDateRecurrence.safeParse(picked)
            if (parsed.success) {
              form.setFieldValue('recurrence', parsed.data)
            }
          }}
        />
      </Input.Wrapper>
      <Group align="flex-start" grow>
        <NumberInput
          label={t(($) => $.automations.date.daysLabel)}
          min={0}
          max={offsetDaysBound(values.sign)}
          allowDecimal={false}
          allowNegative={false}
          {...form.getInputProps('days')}
        />
        <Select
          label={t(($) => $.automations.date.directionLabel)}
          allowDeselect={false}
          data={SIGNS.map((sign) => ({
            value: String(sign),
            label: sign < 0 ? sides.before : sides.after,
          }))}
          {...signProps}
          value={String(values.sign)}
          onChange={(picked) => {
            signProps.onChange(Number(picked) < 0 ? -1 : 1)
          }}
        />
      </Group>
      <LocalTimeInput {...form.getInputProps('localTime')} />
    </Stack>
  )
}

// TimeOfDayForm edits a Wait until's time of day: the local time and the weekdays it counts on,
// none being every day.
export function TimeOfDayForm({
  value,
  onChange,
  validatorRef,
  checked,
  onChecked,
}: MomentFormProps<TimeOfDayDraft>) {
  const { t } = useTranslation()
  const form = useSchemaForm(timeOfDaySchema, value, onChange)
  useValidator(form, validatorRef)
  useShownErrors(form, checked)
  const names: Record<Weekday, string> = {
    monday: t(($) => $.automations.weekday.monday),
    tuesday: t(($) => $.automations.weekday.tuesday),
    wednesday: t(($) => $.automations.weekday.wednesday),
    thursday: t(($) => $.automations.weekday.thursday),
    friday: t(($) => $.automations.weekday.friday),
    saturday: t(($) => $.automations.weekday.saturday),
    sunday: t(($) => $.automations.weekday.sunday),
  }

  return (
    <Stack onBlur={onChecked}>
      <LocalTimeInput {...form.getInputProps('localTime')} />
      <Input.Wrapper
        label={t(($) => $.automations.weekdaysLabel)}
        description={t(($) => $.automations.weekdaysHint)}
      >
        <Chip.Group
          multiple
          value={form.values.weekdays}
          // Held in the week's order, whatever order they were chosen in.
          onChange={(picked) =>
            form.setFieldValue(
              'weekdays',
              WEEKDAYS.filter((day) => picked.includes(day)),
            )
          }
        >
          <Group gap="xs" mt={4}>
            {WEEKDAYS.map((day) => (
              <Chip key={day} value={day} size="xs">
                {names[day]}
              </Chip>
            ))}
          </Group>
        </Chip.Group>
      </Input.Wrapper>
    </Stack>
  )
}
