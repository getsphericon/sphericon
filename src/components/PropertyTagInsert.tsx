import { Button, Group, Popover, Select, Stack, TextInput } from '@mantine/core'
import { getHotkeyHandler } from '@mantine/hooks'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { propertyTag } from '../utils/mergeTags.ts'

// PropertyTagInsert inserts the merge tag of a property of an Event a message binds, by the
// property's name: an Automation's messages bind the Event the Enrollment began with (`event`) and
// each Event an earlier Wait for event arrived on (`awaited.<name>`, ADR 0136). With more than one
// Event, the person picks whose property it is. It shows nothing for a message that binds none.
export function PropertyTagInsert({
  sources,
  disabled = false,
  onInsert,
}: {
  // The Events the message binds, as their merge tags name them, the first picked at first.
  sources: readonly string[]
  disabled?: boolean
  onInsert: (tag: string) => void
}) {
  const { t } = useTranslation()
  const [opened, setOpened] = useState(false)
  const [picked, setPicked] = useState<string | null>(null)
  const [property, setProperty] = useState('')
  const source = picked !== null && sources.includes(picked) ? picked : sources[0]
  if (source === undefined) {
    return null
  }
  const tag = propertyTag(source, property)

  const insert = () => {
    if (tag === null) {
      return
    }
    onInsert(tag)
    setProperty('')
    setOpened(false)
  }

  return (
    <Popover opened={opened} onChange={setOpened} withArrow>
      <Popover.Target>
        <Button
          variant="default"
          size="xs"
          disabled={disabled}
          onClick={() => setOpened((open) => !open)}
        >
          {t(($) => $.propertyTag.open)}
        </Button>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">
          {sources.length > 1 ? (
            <Select
              size="xs"
              label={t(($) => $.propertyTag.sourceLabel)}
              data={[...sources]}
              value={source}
              allowDeselect={false}
              comboboxProps={{ withinPortal: false }}
              onChange={setPicked}
            />
          ) : null}
          <Group gap="xs" align="flex-end">
            <TextInput
              size="xs"
              label={t(($) => $.propertyTag.nameLabel)}
              description={t(($) => $.propertyTag.nameHint)}
              placeholder="amount"
              value={property}
              onChange={(event) => setProperty(event.currentTarget.value)}
              // Made in the handler: onInsert may read a caret ref, which render must not.
              onKeyDown={(event) => getHotkeyHandler([['Enter', insert]])(event)}
            />
            <Button size="xs" disabled={tag === null} onClick={insert}>
              {t(($) => $.propertyTag.insert)}
            </Button>
          </Group>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}
