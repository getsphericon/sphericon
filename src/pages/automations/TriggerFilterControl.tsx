import { Input } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useMount } from 'ahooks'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// The Trigger's filter on the Event's own properties (ADR 0133), edited with the Condition builder
// of the Event filter's place: property tests only, under and, or and not. The node holds it as
// JSON text, as the builder leaves it; a filter still being typed is refused at the Save, and one
// left half-typed is dropped when the panel opens again. The
// `tag` of contact.tagged and contact.untagged offers the Workspace's Tags, the `key` of
// contact.field_changed its Custom fields, which the server checks.
export function TriggerFilterControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const { core } = useJsonForms()
  const node = isRecord(core?.data) ? core.data : {}
  const action = typeof node.triggerEvent === 'string' ? node.triggerEvent : ''
  const text = typeof data === 'string' ? data : ''
  const [initial] = useState(() => filterOf(text) ?? emptyGroup)
  // A filter left half-typed when the panel closed loads as none, so the node drops it too: the
  // builder shows what the Save sends.
  const halfTyped = text !== '' && filterOf(text) === undefined
  useMount(() => {
    if (halfTyped) {
      props.handleChange(path, '')
    }
  })
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
    <Input.Wrapper label={label} description={t(($) => $.automations.triggerFilterHint)}>
      <ConditionBuilder
        workspaceId={workspaceId}
        place="eventFilter"
        initialValue={initial}
        propertyValues={offered}
        onChange={(draft) => props.handleChange(path, filterText(subjectless(draft)))}
      />
    </Input.Wrapper>
  )
}
