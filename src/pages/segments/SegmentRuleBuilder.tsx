import { Button, Group, Stack, Text } from '@mantine/core'
import { QueryBuilderMantine } from '@react-querybuilder/mantine'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { type Field, type Operator, QueryBuilder } from 'react-querybuilder'

import {
  siteCustomFieldsListOptions,
  siteEventsActionsOptions,
  siteSegmentsPreviewMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteSegmentOperator, type SiteSegmentRuleGroup } from '../../generated/site/types.gen.ts'
import { toGroup, toQuery } from './rules.ts'

// What the backend segment engine compiles for each kind of field.
const textOperators: Operator[] = [
  { name: SiteSegmentOperator.EQ, label: 'is' },
  { name: SiteSegmentOperator.NEQ, label: 'is not' },
  { name: SiteSegmentOperator.CONTAINS, label: 'contains' },
  { name: SiteSegmentOperator.BEGINS_WITH, label: 'begins with' },
  { name: SiteSegmentOperator.NULL, label: 'is empty' },
  { name: SiteSegmentOperator.NOT_NULL, label: 'is set' },
]

// A custom field is a key of a JSON column: it can be compared, or tested for membership
// of an array, but not for a prefix or for being empty.
const customFieldOperators: Operator[] = [
  { name: SiteSegmentOperator.EQ, label: 'is' },
  { name: SiteSegmentOperator.NEQ, label: 'is not' },
  { name: SiteSegmentOperator.CONTAINS, label: 'contains' },
  { name: SiteSegmentOperator.NOT_NULL, label: 'is set' },
]

// Event conditions get their own operator set (per-field overrides the global one).
// The value is a day window: blank = ever.
const eventOperators: Operator[] = [
  { name: SiteSegmentOperator.PERFORMED, label: 'performed in last (days)' },
  { name: SiteSegmentOperator.NOT_PERFORMED, label: 'not performed in last (days)' },
]

const contactFields: Field[] = [
  { name: 'subject_id', label: 'Subject ID' },
  { name: 'email', label: 'Email' },
  { name: 'phone', label: 'Phone' },
  { name: 'first_name', label: 'First name' },
  { name: 'last_name', label: 'Last name' },
  { name: 'time_zone', label: 'Time zone' },
]

interface SegmentRuleBuilderProps {
  slug: string
  value: SiteSegmentRuleGroup
  onChange: (group: SiteSegmentRuleGroup) => void
}

// SegmentRuleBuilder is the visual rule editor (react-querybuilder + the official
// Mantine compat package — no hand-rolled condition rows, no custom CSS). It edits the
// API's typed rule group and offers a live preview of how many active contacts match.
export function SegmentRuleBuilder({ slug, value, onChange }: SegmentRuleBuilderProps) {
  const { t } = useTranslation()
  const query = useMemo(() => toQuery(value), [value])
  const preview = useMutation(siteSegmentsPreviewMutation())

  // The workspace's distinct event actions become behavioral fields
  // ("event:<action>") that compile to an EXISTS against the events log.
  const actionsQuery = useQuery(siteEventsActionsOptions({ path: { slug: slug } }))
  // The typed Custom field catalogue (ADR 0006) becomes targetable fields
  // ("custom:<key>") alongside the core fields — one governed attribute list.
  const customFieldsQuery = useQuery(siteCustomFieldsListOptions({ path: { slug: slug } }))
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
      defaultOperator: SiteSegmentOperator.PERFORMED,
      inputType: 'number',
    }))
    return [...contactFields, ...customFields, ...eventFields]
  }, [actionsQuery.data, customFieldsQuery.data])

  return (
    <Stack gap="xs">
      <QueryBuilderMantine>
        <QueryBuilder
          fields={fields}
          operators={textOperators}
          query={query}
          enableMountQueryChange={false}
          onQueryChange={(q) => onChange(toGroup(q))}
        />
      </QueryBuilderMantine>
      <Group>
        <Button
          variant="light"
          size="compact-sm"
          loading={preview.isPending}
          onClick={() => preview.mutate({ path: { slug: slug }, body: { definition: value } })}
        >
          {t(($) => $.segments.previewButton)}
        </Button>
        {preview.data ? (
          <Text size="sm">{t(($) => $.segments.previewCount, { count: preview.data.count })}</Text>
        ) : null}
      </Group>
    </Stack>
  )
}
