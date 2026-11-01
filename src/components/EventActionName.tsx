import { Badge, Tooltip } from '@mantine/core'
import type { MantineColor } from '@mantine/core'
import { Fragment } from 'react'

import { useEventActionLabel } from '../hooks/useEventActionLabel.ts'
import { ActionName, ActionOption } from './ActionName.tsx'

// EventActionName shows an Event action by its words; a system action keeps its code on hover, the
// name the API, segments and webhooks know it by.
export function EventActionName({ action }: { action: string }) {
  return <ActionName action={action} label={useEventActionLabel()(action)} />
}

// EventActionNames shows several actions in a row, separated by commas.
export function EventActionNames({ actions }: { actions: string[] }) {
  return actions.map((action, index) => (
    <Fragment key={action}>
      {index > 0 ? ', ' : null}
      <EventActionName action={action} />
    </Fragment>
  ))
}

// EventActionBadge is an action as a badge, the code on hover as EventActionName's.
export function EventActionBadge({ action, color }: { action: string; color: MantineColor }) {
  const label = useEventActionLabel()(action)
  const badge = (
    <Badge variant="outline" color={color}>
      {label}
    </Badge>
  )
  return label === action ? badge : <Tooltip label={action}>{badge}</Tooltip>
}

// EventActionOption is a dropdown option of an action field whose value is the code itself (an
// Autocomplete, a TagsInput): the words, then the code it puts in the field.
export function EventActionOption({ action }: { action: string }) {
  return <ActionOption action={action} label={useEventActionLabel()(action)} />
}
