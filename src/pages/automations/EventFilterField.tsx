import { useState } from 'react'

import { emptyGroup } from '../../components/conditions/condition.ts'
import { EventFilterBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { filterOf, filterText } from './definition.ts'

// The filter of an Event matcher on the action (ADR 0133), edited with the builder of an Event
// filter: property tests, and on a message's System event what sent it, under and, or and not. It
// loads the filter's text once (a complete filter, or none for one still being typed) and hands
// back the text of what the builder holds (filterText).
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

  return (
    <EventFilterBuilder
      workspaceId={workspaceId}
      action={action}
      initialValue={initial}
      onChange={(draft) => onChange(filterText(draft))}
    />
  )
}
