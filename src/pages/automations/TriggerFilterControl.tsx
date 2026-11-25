import { Input } from '@mantine/core'
import { useMount } from 'ahooks'
import { useTranslation } from 'react-i18next'

import { filterOf } from './definition.ts'
import { EventFilterField } from './EventFilterField.tsx'

// The Trigger's filter on the Event's own properties (ADR 0133), edited with the Event filter field.
// The flow holds it as JSON text, as the builder leaves it; a filter still being typed is refused at
// the Save, and one left half-typed is dropped when the panel opens again.
export function TriggerFilterControl({
  action,
  text,
  onChange,
}: {
  action: string
  text: string
  onChange: (text: string) => void
}) {
  const { t } = useTranslation()
  // A filter left half-typed when the panel closed loads as none, so the flow drops it too: the
  // builder shows what the Save sends.
  useMount(() => {
    if (text !== '' && filterOf(text) === undefined) {
      onChange('')
    }
  })

  return (
    <Input.Wrapper
      label={t(($) => $.automations.triggerFilterLabel)}
      description={t(($) => $.automations.triggerFilterHint)}
    >
      <EventFilterField action={action} text={text} onChange={onChange} />
    </Input.Wrapper>
  )
}
