import { Autocomplete } from '@mantine/core'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useTranslation } from 'react-i18next'

import { TRIGGER_EVENTS } from './nodes.ts'

// The Trigger is any Event action a Workspace records, so the field takes free text and offers
// the built-in events as suggestions.
export function TriggerControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()

  return (
    <Autocomplete
      label={label}
      description={t(($) => $.automations.triggerHint)}
      data={[...TRIGGER_EVENTS]}
      value={typeof data === 'string' ? data : ''}
      onChange={(value) => props.handleChange(path, value.trim())}
    />
  )
}
