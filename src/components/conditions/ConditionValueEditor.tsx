import { Autocomplete, Group, TextInput } from '@mantine/core'
import { MantineValueEditor } from '@react-querybuilder/mantine'
import type { MantineValueEditorProps } from '@react-querybuilder/mantine'
import { useTranslation } from 'react-i18next'

import {
  eventFieldPrefix,
  isPropertyField,
  namedOperator,
  propertyValue,
  propertyValueOf,
} from './condition.ts'
import { EventCountEditor } from './EventCountEditor.tsx'
import { isPresence } from './fieldOperators.ts'
import type { ValueEditorContext } from './valueEditorContext.ts'

// ConditionValueEditor is the builder's value editor: an event field edits its count and window, a
// property of the triggering or an awaited Event its key and the value its test compares with, and every other field keeps the
// Mantine editor.
export function ConditionValueEditor(props: MantineValueEditorProps) {
  if (props.field.startsWith(eventFieldPrefix)) {
    return <EventCountEditor {...props} />
  }
  if (isPropertyField(props.field)) {
    return <PropertyEditor {...props} />
  }
  return <MantineValueEditor {...props} />
}

// PropertyEditor edits the key of the Event's property in an input of its own, with the keys the
// builder suggests, then the value its test compares with; a presence test compares with none. A
// blank key stays blank and is marked required.
function PropertyEditor(props: MantineValueEditorProps) {
  const { t } = useTranslation()
  const context: ValueEditorContext = props.context ?? {}
  const { key, value } = propertyValue(props.value)
  return (
    <Group gap="xs" wrap="nowrap" align="flex-start">
      {context.propertyKeys === undefined || context.propertyKeys.length === 0 ? (
        <TextInput
          aria-label={t(($) => $.segments.propertyKeyLabel)}
          placeholder={t(($) => $.segments.propertyKeyLabel)}
          value={key}
          error={key === '' ? t(($) => $.validation.required) : undefined}
          onChange={(event) =>
            props.handleOnChange(propertyValueOf(event.currentTarget.value, value))
          }
        />
      ) : (
        <Autocomplete
          aria-label={t(($) => $.segments.propertyKeyLabel)}
          placeholder={t(($) => $.segments.propertyKeyLabel)}
          data={[...context.propertyKeys]}
          value={key}
          error={key === '' ? t(($) => $.validation.required) : undefined}
          onChange={(next) => props.handleOnChange(propertyValueOf(next, value))}
        />
      )}
      {isPresence(props.operator) ? null : (
        <PropertyValueEditor {...props} propertyKey={key} value={value} />
      )}
    </Group>
  )
}

// PropertyValueEditor edits the value a property's test compares with: for a text test of a key the
// builder offers values for (ValueEditorContext), free text with those values as suggestions, else
// the Mantine editor.
function PropertyValueEditor(props: MantineValueEditorProps & { propertyKey: string }) {
  const { propertyKey, value, ...rest } = props
  const context: ValueEditorContext = rest.context ?? {}
  const offered = context.propertyValues?.[propertyKey]
  const change = (next: unknown) => rest.handleOnChange(propertyValueOf(propertyKey, next))
  if (offered !== undefined && namedOperator(rest.operator)?.kind === 'text') {
    return (
      <Autocomplete
        aria-label={rest.title}
        data={[...offered]}
        value={typeof value === 'string' ? value : ''}
        onChange={change}
      />
    )
  }
  return <MantineValueEditor {...rest} value={value} handleOnChange={change} />
}
