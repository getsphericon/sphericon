import { Autocomplete } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { EventActionOption } from '../../components/EventActionName.tsx'
import { useEventActionFilter } from '../../hooks/useEventActionLabel.ts'
import { TRIGGER_EVENTS } from './triggerEvents.ts'

// The Trigger's action is any Event action a Workspace records, so the field takes free text and
// offers the built-in events as suggestions.
export function TriggerControl({
  value,
  onChange,
}: {
  value: string
  onChange: (action: string) => void
}) {
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()

  return (
    <Autocomplete
      label={t(($) => $.automations.triggerLabel)}
      description={t(($) => $.automations.triggerHint)}
      data={[...TRIGGER_EVENTS]}
      renderOption={({ option }) => <EventActionOption action={option.value} />}
      filter={actionFilter}
      value={value}
      onChange={(action) => onChange(action.trim())}
    />
  )
}
