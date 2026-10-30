import { Group, NumberInput } from '@mantine/core'
import { MantineValueEditor } from '@react-querybuilder/mantine'
import type { MantineValueEditorProps } from '@react-querybuilder/mantine'
import { useTranslation } from 'react-i18next'

import { zSiteConditionEventLeaf } from '../../generated/site/zod.gen.ts'
import { eventCount, eventCountOf, eventFieldPrefix, isCountOperator } from './condition.ts'

// The least count and window the contract takes (atLeast's own floor of one is the server's), as
// the inputs' min.
function least(min: number | null): { min?: number } {
  return min === null ? {} : { min }
}
const leastTimes = least(zSiteConditionEventLeaf.shape.times.minValue)
const leastDays = least(zSiteConditionEventLeaf.shape.withinDays.unwrap().minValue)

// A typed number, or nothing for a cleared input.
function typed(input: unknown): number | undefined {
  return typeof input === 'number' ? input : undefined
}

// EventCountEditor is the builder's value editor: an event field edits its count (for atLeast
// and atMost; a preset has its own) and its window in days, blank for all time. A blank count
// stays blank and is marked required. Every other field keeps the Mantine editor.
export function EventCountEditor(props: MantineValueEditorProps) {
  const { t } = useTranslation()
  if (!props.field.startsWith(eventFieldPrefix)) {
    return <MantineValueEditor {...props} />
  }
  const count = eventCount(props.value)
  return (
    <Group gap="xs" wrap="nowrap" align="flex-start">
      {isCountOperator(props.operator) ? (
        <NumberInput
          aria-label={t(($) => $.segments.eventTimesLabel)}
          placeholder={t(($) => $.segments.eventTimesLabel)}
          {...leastTimes}
          allowDecimal={false}
          allowNegative={false}
          value={count.times ?? ''}
          error={count.times === undefined ? t(($) => $.validation.required) : undefined}
          onChange={(input) => props.handleOnChange(eventCountOf(typed(input), count.withinDays))}
        />
      ) : null}
      <NumberInput
        aria-label={t(($) => $.segments.eventWindowLabel)}
        placeholder={t(($) => $.segments.eventWindowEver)}
        {...leastDays}
        allowDecimal={false}
        allowNegative={false}
        value={count.withinDays ?? ''}
        onChange={(input) => props.handleOnChange(eventCountOf(count.times, typed(input)))}
      />
    </Group>
  )
}
