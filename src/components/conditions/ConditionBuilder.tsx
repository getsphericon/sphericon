import { Button, Group, Stack, Text } from '@mantine/core'
import { QueryBuilderMantine } from '@react-querybuilder/mantine'
import { useMutation, useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QueryBuilder } from 'react-querybuilder'
import type { Field, Operator } from 'react-querybuilder'

import {
  siteCustomFieldsListOptions,
  siteEventsActionsOptions,
  siteSegmentsPreviewMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  ContactAttribute,
  EventCountOperator,
  HasOperator,
} from '../../generated/site/types.gen.ts'
import type { RuleCondition, SegmentCondition } from '../../generated/site/types.gen.ts'
import { useEventActionLabel } from '../../hooks/useEventActionLabel.ts'
import { useTimeZone } from '../../hooks/useFormat.ts'
import {
  customFieldId,
  eventFieldPrefix,
  eventPresets,
  subjectFields,
  toCondition,
  toDraft,
  toQuery,
  unnamed,
} from './condition.ts'
import type { ConditionDraft, ConditionPlace } from './condition.ts'
import { ConditionValueEditor } from './ConditionValueEditor.tsx'
import {
  attributeOperators,
  birthdayOperators,
  haveOperators,
  inputTypeOf,
  inputTypeOfField,
  operatorsOf,
  propertyOperators,
  tagOperators,
} from './fieldOperators.ts'

// Event conditions count the action (per-field overrides the global operator set): the presets
// "performed" and "not performed", or at least / at most a number of times. EventCountEditor
// edits the count and the window in days (blank = ever).
function eventOperators(t: TFunction): Operator[] {
  return [
    { name: eventPresets.performed, label: t(($) => $.conditions.eventOperators.performed) },
    { name: eventPresets.notPerformed, label: t(($) => $.conditions.eventOperators.notPerformed) },
    { name: EventCountOperator.AT_LEAST, label: t(($) => $.conditions.eventOperators.atLeast) },
    { name: EventCountOperator.AT_MOST, label: t(($) => $.conditions.eventOperators.atMost) },
  ]
}

// The Contact's leaves: its text attributes, its birthday, its Tags, and whether it can be reached
// on Telegram or by SMS.
function contactFields(t: TFunction): Field[] {
  return [
    ...Object.values(ContactAttribute).map((attribute) => ({
      name: attribute,
      label: t(($) => $.conditions.attributes[attribute]),
      operators: attributeOperators(t),
    })),
    {
      name: subjectFields.birthday,
      label: t(($) => $.conditions.fields.birthday),
      operators: birthdayOperators(t),
    },
    // Whether the Contact has the Tag typed as the value.
    {
      name: subjectFields.tag,
      label: t(($) => $.conditions.fields.tag),
      operators: tagOperators(t),
    },
    // Whether a Telegram chat is bound to the Contact or its Visitors: no value to type.
    {
      name: subjectFields.telegram,
      label: t(($) => $.conditions.fields.telegram),
      operators: haveOperators(t),
      defaultOperator: HasOperator.HAS,
    },
    // Whether the Contact's phone can be texted (it has an SMS Confirmation and no SMS opt-out): no
    // value to type.
    {
      name: subjectFields.sms,
      label: t(($) => $.conditions.fields.sms),
      operators: haveOperators(t),
      defaultOperator: HasOperator.HAS,
    },
  ]
}

// A Rule's Condition also tests a property of the Event that fired it, its key typed beside it.
function placeFields(t: TFunction, place: ConditionPlace): Field[] {
  switch (place) {
    case 'segment':
      return []
    case 'rule':
      return [
        {
          name: subjectFields.property,
          label: t(($) => $.conditions.fields.property),
          operators: propertyOperators(t),
        },
      ]
    default:
      return unnamed(place)
  }
}

interface ConditionBuilderProps {
  workspaceId: string
  // The Condition the builder starts from. The builder keeps its own state from then on (an
  // operator the user picked stays picked, a count can be blank while it is typed), so a later
  // change of this value is not read: a form that loads another Condition remounts the builder.
  initialValue: SegmentCondition | RuleCondition
  // Every edit, as a draft: toCondition is undefined while an event count, a number, a day or a
  // property's key is blank.
  onChange: (draft: ConditionDraft) => void
  // Whose Condition it is (ADR 0181): the builder offers what the place admits. Only a Rule has an
  // Event, whose properties it tests; only a Segment's Condition is previewed.
  place: ConditionPlace
}

// ConditionBuilder is the visual Condition editor (react-querybuilder + the official
// Mantine compat package — no hand-rolled condition rows, no custom CSS). It edits the
// API's typed rule group and offers a live preview of how many active contacts match.
export function ConditionBuilder({
  workspaceId,
  initialValue,
  onChange,
  place,
}: ConditionBuilderProps) {
  const { t } = useTranslation()
  const labelOf = useEventActionLabel()
  // The days of before and after are days in the User time zone (ADR 0068).
  const timeZone = useTimeZone()
  const [initialQuery] = useState(() => toQuery(initialValue, timeZone))
  const [draft, setDraft] = useState<ConditionDraft>(initialValue)
  const condition = toCondition(draft, 'segment')
  const preview = useMutation(siteSegmentsPreviewMutation())

  // The workspace's distinct event actions become behavioral fields ("event:<action>"), each an
  // event leaf: a count of the action in the events log.
  const actionsQuery = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  // The typed Custom field catalogue (ADR 0006) becomes targetable fields alongside the core
  // fields — one governed attribute list. Each offers the operators the server lists for its type,
  // per typed test; the operator picks the value input (inputTypeOf), a number field's own is a
  // number.
  const customFieldsQuery = useQuery(siteCustomFieldsListOptions({ path: { workspaceId } }))
  const fields = useMemo<Field[]>(() => {
    const customFields = (customFieldsQuery.data?.items ?? []).map<Field>((f) => ({
      name: customFieldId(f.key),
      label: f.name,
      operators: operatorsOf(t, f),
      inputType: inputTypeOfField(f),
    }))
    // A System event reads as its name ("Opened an email"); a customer's action as it was tracked.
    const eventFields = (actionsQuery.data?.actions ?? []).map<Field>((action) => ({
      name: `${eventFieldPrefix}${action}`,
      label:
        labelOf(action) === action ? t(($) => $.segments.didEvent, { action }) : labelOf(action),
      operators: eventOperators(t),
      defaultOperator: eventPresets.performed,
    }))
    return [...contactFields(t), ...customFields, ...eventFields, ...placeFields(t, place)]
  }, [actionsQuery.data, customFieldsQuery.data, labelOf, t, place])

  return (
    <Stack gap="xs">
      <QueryBuilderMantine>
        <QueryBuilder
          fields={fields}
          operators={attributeOperators(t)}
          getInputType={(_field, operator, { fieldData }) => inputTypeOf(operator, fieldData)}
          defaultQuery={initialQuery}
          enableMountQueryChange={false}
          controlElements={{ valueEditor: ConditionValueEditor }}
          onQueryChange={(q) => {
            const next = toDraft(q, timeZone)
            setDraft(next)
            onChange(next)
          }}
        />
      </QueryBuilderMantine>
      {place === 'segment' ? (
        <Group>
          <Button
            variant="light"
            size="compact-sm"
            loading={preview.isPending}
            disabled={condition === undefined}
            onClick={() => {
              if (condition) {
                preview.mutate({ path: { workspaceId }, body: { condition } })
              }
            }}
          >
            {t(($) => $.segments.previewButton)}
          </Button>
          {preview.data ? (
            <Text size="sm">
              {t(($) => $.segments.previewCount, { count: preview.data.count })}
            </Text>
          ) : null}
        </Group>
      ) : null}
    </Stack>
  )
}
