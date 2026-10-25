import { Button, Group, Stack, Text } from '@mantine/core'
import { QueryBuilderMantine } from '@react-querybuilder/mantine'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { QueryBuilder } from 'react-querybuilder'
import type { Field, Operator } from 'react-querybuilder'

import {
  siteCustomFieldsListOptions,
  siteEventsActionsOptions,
  siteSegmentsPreviewMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteConditionOperator } from '../../generated/site/types.gen.ts'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { toGroup, toQuery } from './condition.ts'
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

// Event conditions get their own operator set (per-field overrides the global one).
// The value is a day window: blank = ever.
const eventOperators: Operator[] = [
  { name: SiteConditionOperator.PERFORMED, label: 'performed in last (days)' },
  { name: SiteConditionOperator.NOT_PERFORMED, label: 'not performed in last (days)' },
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
  value: SiteConditionGroup
  onChange: (group: SiteConditionGroup) => void
  // Offer the tests on the triggering Event's properties. Only a Rule has an Event: a
  // Segment refuses them, and so does the segment preview, which is hidden with them.
  triggerLeaves?: boolean
}

// ConditionBuilder is the visual Condition editor (react-querybuilder + the official
// Mantine compat package — no hand-rolled condition rows, no custom CSS). It edits the
// API's typed rule group and offers a live preview of how many active contacts match.
export function ConditionBuilder({
  workspaceId,
  value,
  onChange,
  triggerLeaves = false,
}: ConditionBuilderProps) {
  const { t } = useTranslation()
  const query = useMemo(() => toQuery(value), [value])
  const preview = useMutation(siteSegmentsPreviewMutation())

  // The workspace's distinct event actions become behavioral fields
  // ("event:<action>") that compile to an EXISTS against the events log.
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
      name: `event:${action}`,
      label: `Did "${action}"`,
      operators: eventOperators,
      defaultOperator: SiteConditionOperator.PERFORMED,
      inputType: 'number',
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
          query={query}
          enableMountQueryChange={false}
          controlElements={triggerLeaves ? { fieldSelector: TriggerFieldSelector } : {}}
          onQueryChange={(q) => onChange(toGroup(q))}
        />
      </QueryBuilderMantine>
      {triggerLeaves ? null : (
        <Group>
          <Button
            variant="light"
            size="compact-sm"
            loading={preview.isPending}
            onClick={() => preview.mutate({ path: { workspaceId }, body: { condition: value } })}
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
