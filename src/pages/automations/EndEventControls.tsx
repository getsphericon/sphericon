import { Autocomplete, TagsInput } from '@mantine/core'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useTranslation } from 'react-i18next'

import { EventActionOption } from '../../components/EventActionName.tsx'
import { useEventActionFilter } from '../../hooks/useEventActionLabel.ts'
import { exitEventsOf } from './definition.ts'
import { TRIGGER_EVENTS } from './nodes.ts'

// The exit events end an active Enrollment early (#751). The node holds them one per line; the
// field shows them as tags, any Event action, with the built-in events as suggestions.
export function ExitEventsControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()

  return (
    <TagsInput
      label={label}
      description={t(($) => $.automations.exitEventsHint)}
      data={[...TRIGGER_EVENTS]}
      renderOption={({ option }) => <EventActionOption action={option.value} />}
      filter={actionFilter}
      value={exitEventsOf(typeof data === 'string' ? data : '')}
      onChange={(next) => props.handleChange(path, next.join('\n'))}
    />
  )
}

// The goal event ends an active Enrollment as having reached its goal; empty is none.
export function GoalEventControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()

  return (
    <Autocomplete
      label={label}
      description={t(($) => $.automations.goalEventHint)}
      data={[...TRIGGER_EVENTS]}
      renderOption={({ option }) => <EventActionOption action={option.value} />}
      filter={actionFilter}
      value={typeof data === 'string' ? data : ''}
      onChange={(value) => props.handleChange(path, value.trim())}
    />
  )
}
