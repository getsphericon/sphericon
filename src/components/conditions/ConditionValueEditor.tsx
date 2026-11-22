import { Group, TextInput } from '@mantine/core'
import { MantineValueEditor } from '@react-querybuilder/mantine'
import type { MantineValueEditorProps } from '@react-querybuilder/mantine'
import { useTranslation } from 'react-i18next'

import { eventFieldPrefix, propertyValue, propertyValueOf, subjectFields } from './condition.ts'
import { EventCountEditor } from './EventCountEditor.tsx'
import { isPresence } from './fieldOperators.ts'

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

// PropertyEditor edits the key of the triggering Event's property in an input of its own, then the
// value its test compares with; a presence test compares with none. A blank key stays blank and is
// marked required.
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
        <MantineValueEditor
          {...props}
          value={value}
          handleOnChange={(next: unknown) => props.handleOnChange(propertyValueOf(key, next))}
        />
      )}
    </Group>
  )
}
