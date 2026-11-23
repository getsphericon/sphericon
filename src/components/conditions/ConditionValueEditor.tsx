import { Autocomplete, Group, TextInput } from '@mantine/core'
import { MantineValueEditor } from '@react-querybuilder/mantine'
import type { MantineValueEditorProps } from '@react-querybuilder/mantine'
import { useTranslation } from 'react-i18next'

import {
  eventFieldPrefix,
  namedOperator,
  propertyValue,
  propertyValueOf,
  subjectFields,
} from './condition.ts'
import { EventCountEditor } from './EventCountEditor.tsx'
import { isPresence } from './fieldOperators.ts'

// The values offered for a property's text test, by the property's key.
export type PropertyValues = Partial<Record<string, readonly string[]>>

// What the builder hands its value editor through the query builder's context.
export interface ValueEditorContext {
  propertyValues?: PropertyValues
}

// ConditionValueEditor is the builder's value editor: an event field edits its count and window, an
// Event property its key and the value its test compares with, and every other field keeps the
// Mantine editor.
export function ConditionValueEditor(props: MantineValueEditorProps) {
  if (props.field.startsWith(eventFieldPrefix)) {
    return <EventCountEditor {...props} />
  }
  if (props.field === subjectFields.property) {
    return <PropertyEditor {...props} />
  }
  return <MantineValueEditor {...props} />
}

// PropertyEditor edits the key of the Event's property in an input of its own, then the value its
// test compares with; a presence test compares with none. A blank key stays blank and is marked
// required.
function PropertyEditor(props: MantineValueEditorProps) {
  const { t } = useTranslation()
  const { key, value } = propertyValue(props.value)
  return (
    <Group gap="xs" wrap="nowrap" align="flex-start">
      <TextInput
        aria-label={t(($) => $.segments.propertyKeyLabel)}
        placeholder={t(($) => $.segments.propertyKeyLabel)}
        value={key}
        error={key === '' ? t(($) => $.validation.required) : undefined}
        onChange={(event) =>
          props.handleOnChange(propertyValueOf(event.currentTarget.value, value))
        }
      />
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
