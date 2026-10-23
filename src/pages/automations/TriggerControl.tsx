import { Autocomplete } from '@mantine/core'
import type { ControlProps } from '@workflowbuilder/sdk'
import { I18nextProvider, useTranslation } from 'react-i18next'

import { i18n } from '../../i18n.ts'
import { TRIGGER_EVENTS } from './nodes.ts'

// The Trigger is any Event action a Workspace records, so the field takes free text and offers
// the built-in events as suggestions.
function TriggerControl(props: ControlProps) {
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

// The builder subtree reads the global i18next instance (see AutomationBuilder); the control is
// the app's own UI, so it is bound back to the app's instance.
export function TriggerControlInApp(props: ControlProps) {
  return (
    <I18nextProvider i18n={i18n}>
      <TriggerControl {...props} />
    </I18nextProvider>
  )
}
