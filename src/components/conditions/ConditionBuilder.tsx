import { Button, Group, Stack, Text } from '@mantine/core'
import { QueryBuilderMantine } from '@react-querybuilder/mantine'
import { useMutation, useQuery } from '@tanstack/react-query'
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
  SiteConditionEventOperator,
  SiteConditionOperator,
} from '../../generated/site/types.gen.ts'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { eventFieldPrefix, eventPresets, toCondition, toDraft, toQuery } from './condition.ts'
import type { ConditionDraft } from './condition.ts'
import { EventCountEditor } from './EventCountEditor.tsx'
import { triggerFieldPrefix } from './triggerFieldPrefix.ts'
import { TriggerFieldSelector } from './TriggerFieldSelector.tsx'

// What the backend segment engine compiles for each kind of field.
const textOperators: Operator[] = [
  { name: SiteConditionOperator.EQ, label: 'is' },
  { name: SiteConditionOperator.NEQ, label: 'is not' },
  { name: SiteConditionOperator.CONTAINS, label: 'contains' },
  { name: SiteConditionOperator.BEGINS_WITH, label: 'begins with' },
  { name: SiteConditionOperator.NULL, label: 'is empty' },
  { name: SiteConditionOperator.NOT_NULL, label: 'is set' },
]

// A custom field is a key of a JSON column: it can be compared, or tested for membership
// of an array, but not for a prefix or for being empty.
const customFieldOperators: Operator[] = [
  { name: SiteConditionOperator.EQ, label: 'is' },
  { name: SiteConditionOperator.NEQ, label: 'is not' },
  { name: SiteConditionOperator.CONTAINS, label: 'contains' },
  { name: SiteConditionOperator.NOT_NULL, label: 'is set' },
]

// Event conditions count the action (per-field overrides the global operator set): the presets
// "performed" and "not performed", or at least / at most a number of times. EventCountEditor
// edits the count and the window in days (blank = ever).
const eventOperators: Operator[] = [
  { name: eventPresets.performed, label: 'performed' },
  { name: eventPresets.notPerformed, label: 'not performed' },
  { name: SiteConditionEventOperator.AT_LEAST, label: 'at least (times)' },
  { name: SiteConditionEventOperator.AT_MOST, label: 'at most (times)' },
]

const contactFields: Field[] = [
  { name: 'subject_id', label: 'Subject ID' },
  { name: 'email', label: 'Email' },
  { name: 'phone', label: 'Phone' },
  { name: 'first_name', label: 'First name' },
  { name: 'last_name', label: 'Last name' },
  { name: 'time_zone', label: 'Time zone' },
]

interface ConditionBuilderProps {
  workspaceId: string
  // The Condition the builder starts from. The builder keeps its own state from then on (an
  // operator the user picked stays picked, a count can be blank while it is typed), so a later
  // change of this value is not read: a form that loads another Condition remounts the builder.
  initialValue: SiteConditionGroup
  // Every edit, as a draft: toCondition is undefined while an event count is blank.
  onChange: (draft: ConditionDraft) => void
  // Offer the tests on the triggering Event's properties. Only a Rule has an Event: a
  // Segment refuses them, and so does the segment preview, which is hidden with them.
  triggerLeaves?: boolean
}

// ConditionBuilder is the visual Condition editor (react-querybuilder + the official
// Mantine compat package — no hand-rolled condition rows, no custom CSS). It edits the
// API's typed rule group and offers a live preview of how many active contacts match.
export function ConditionBuilder({
  workspaceId,
  initialValue,
  onChange,
  triggerLeaves = false,
}: ConditionBuilderProps) {
  const { t } = useTranslation()
  const [initialQuery] = useState(() => toQuery(initialValue))
  const [draft, setDraft] = useState<ConditionDraft>(initialValue)
  const condition = toCondition(draft)
  const preview = useMutation(siteSegmentsPreviewMutation())

  // The workspace's distinct event actions become behavioral fields ("event:<action>"), each an
  // event leaf: a count of the action in the events log.
  const actionsQuery = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  // The typed Custom field catalogue (ADR 0006) becomes targetable fields
  // ("custom:<key>") alongside the core fields — one governed attribute list.
  const customFieldsQuery = useQuery(siteCustomFieldsListOptions({ path: { workspaceId } }))
  const fields = useMemo<Field[]>(() => {
    const customFields = (customFieldsQuery.data?.items ?? []).map<Field>((f) => ({
      name: `custom:${f.key}`,
      label: f.name,
      operators: customFieldOperators,
      inputType: f.type === 'number' ? 'number' : 'text',
    }))
    const eventFields = (actionsQuery.data?.actions ?? []).map<Field>((action) => ({
      name: `${eventFieldPrefix}${action}`,
      label: `Did "${action}"`,
      operators: eventOperators,
      defaultOperator: eventPresets.performed,
    }))
    const triggerFields: Field[] = triggerLeaves
      ? [{ name: triggerFieldPrefix, label: 'Event property' }]
      : []
    return [...contactFields, ...customFields, ...eventFields, ...triggerFields]
  }, [actionsQuery.data, customFieldsQuery.data, triggerLeaves])

  return (
    <Stack gap="xs">
      <QueryBuilderMantine>
        <QueryBuilder
          fields={fields}
          operators={textOperators}
          defaultQuery={initialQuery}
          enableMountQueryChange={false}
          controlElements={{
            valueEditor: EventCountEditor,
            ...(triggerLeaves ? { fieldSelector: TriggerFieldSelector } : {}),
          }}
          onQueryChange={(q) => {
            const next = toDraft(q)
            setDraft(next)
            onChange(next)
          }}
        />
      </QueryBuilderMantine>
      {triggerLeaves ? null : (
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
      )}
    </Stack>
  )
}
