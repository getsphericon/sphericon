import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import { emptyGroup, subjectless } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import type { PropertyValues } from '../../components/conditions/ConditionValueEditor.tsx'
import {
  siteCustomFieldsListOptions,
  siteTagsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SystemEventAction } from '../../generated/site/types.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { filterOf, filterText } from './definition.ts'

// How many Tags the filter offers: the catalogue's first page, as a suggestion.
const TAG_SUGGESTIONS = 100

// The actions whose `tag` names a Tag of the catalogue.
const TAG_ACTIONS = new Set<string>([
  SystemEventAction.CONTACT_TAGGED,
  SystemEventAction.CONTACT_UNTAGGED,
])

// The filter of an Event matcher on the action (ADR 0133), edited with the Condition builder of the
// Event filter's place: property tests only, under and, or and not. It loads the filter's text once
// (a complete filter, or none for one still being typed) and hands back the text of what the
// builder holds (filterText). The `tag` of contact.tagged and contact.untagged offers the
// Workspace's Tags, the `key` of contact.field_changed its Custom fields, which the server checks.
export function EventFilterField({
  action,
  text,
  onChange,
}: {
  action: string
  text: string
  onChange: (text: string) => void
}) {
  const { workspaceId } = workspaceRoute.useParams()
  const [initial] = useState(() => filterOf(text) ?? emptyGroup)
  const tags = useQuery({
    ...siteTagsListOptions({ path: { workspaceId }, query: { pageSize: TAG_SUGGESTIONS } }),
    enabled: TAG_ACTIONS.has(action),
  })
  const fields = useQuery({
    ...siteCustomFieldsListOptions({ path: { workspaceId } }),
    enabled: action === SystemEventAction.CONTACT_FIELD_CHANGED,
  })
  const offered: PropertyValues = {}
  if (TAG_ACTIONS.has(action)) {
    offered.tag = (tags.data?.items ?? []).map((tag) => tag.name)
  }
  if (action === SystemEventAction.CONTACT_FIELD_CHANGED) {
    offered.key = (fields.data?.items ?? []).map((field) => field.key)
  }

  return (
    <ConditionBuilder
      workspaceId={workspaceId}
      place="eventFilter"
      initialValue={initial}
      propertyValues={offered}
      onChange={(draft) => onChange(filterText(subjectless(draft)))}
    />
  )
}
