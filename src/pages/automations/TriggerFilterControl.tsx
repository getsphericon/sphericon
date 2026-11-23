import { Input } from '@mantine/core'
import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useMount } from 'ahooks'
import { useTranslation } from 'react-i18next'

import { filterOf } from './definition.ts'
import { EventFilterField } from './EventFilterField.tsx'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// The Trigger's filter on the Event's own properties (ADR 0133), edited with the Event filter field.
// The node holds it as JSON text, as the builder leaves it; a filter still being typed is refused at
// the Save, and one left half-typed is dropped when the panel opens again.
export function TriggerFilterControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const { core } = useJsonForms()
  const node = isRecord(core?.data) ? core.data : {}
  const action = typeof node.triggerEvent === 'string' ? node.triggerEvent : ''
  const text = typeof data === 'string' ? data : ''
  // A filter left half-typed when the panel closed loads as none, so the node drops it too: the
  // builder shows what the Save sends.
  const halfTyped = text !== '' && filterOf(text) === undefined
  useMount(() => {
    if (halfTyped) {
      props.handleChange(path, '')
    }
  })

  return (
    <Input.Wrapper label={label} description={t(($) => $.automations.triggerFilterHint)}>
      <EventFilterField
        action={action}
        text={text}
        onChange={(next) => props.handleChange(path, next)}
      />
    </Input.Wrapper>
  )
}
