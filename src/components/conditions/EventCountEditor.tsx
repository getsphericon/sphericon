import { Button, Group, Modal, NumberInput, Stack, Text } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import type { MantineValueEditorProps } from '@react-querybuilder/mantine'
import { useTranslation } from 'react-i18next'

import { zEventLeafInput } from '../../generated/site/zod.gen.ts'
import {
  emptyGroup,
  eventCount,
  eventCountOf,
  eventFieldPrefix,
  isCountOperator,
  reachesPastKept,
  toCondition,
} from './condition.ts'
import type { ConditionDraft } from './condition.ts'
import type { ValueEditorContext } from './valueEditorContext.ts'

// The least count and window the contract takes (atLeast's own floor of one is the server's), as
// the inputs' min.
function least(min: number | null): { min?: number } {
  return min === null ? {} : { min }
}
const leastTimes = least(zEventLeafInput.shape.times.minValue)
const leastDays = least(zEventLeafInput.shape.withinDays.unwrap().minValue)

// A typed number, or nothing for a cleared input.
function typed(input: unknown): number | undefined {
  return typeof input === 'number' ? input : undefined
}

// EventCountEditor is the value editor of an event field: its count (for atLeast and atMost; a
// preset has its own), its window in days, blank for all time, and the filter of the Events it
// counts, edited in a modal. A blank count stays blank and is marked required. A count that reaches
// past the Events kept says how far back they are.
export function EventCountEditor(props: MantineValueEditorProps) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const context: ValueEditorContext = props.context ?? {}
  const action = props.field.slice(eventFieldPrefix.length)
  const count = eventCount(props.value)
  const change = (times?: number, withinDays?: number, filter?: ConditionDraft) =>
    props.handleOnChange(eventCountOf(times, withinDays, filter))
  const FilterEditor = context.eventFilterEditor
  const keptDays = context.eventsKeptDays ?? 0
  const tests = count.filter?.entries.length ?? 0
  return (
    <Stack gap={4}>
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
            onChange={(input) => change(typed(input), count.withinDays, count.filter)}
          />
        ) : null}
        <NumberInput
          aria-label={t(($) => $.segments.eventWindowLabel)}
          placeholder={t(($) => $.segments.eventWindowEver)}
          {...leastDays}
          allowDecimal={false}
          allowNegative={false}
          value={count.withinDays ?? ''}
          onChange={(input) => change(count.times, typed(input), count.filter)}
        />
        {FilterEditor !== undefined && context.workspaceId !== undefined ? (
          <Button variant="default" onClick={open}>
            {tests === 0
              ? t(($) => $.segments.eventFilterButton)
              : t(($) => $.segments.eventFilterTests, { count: tests })}
          </Button>
        ) : null}
      </Group>
      {reachesPastKept({
        action,
        withinDays: count.withinDays,
        keptDays,
        keptActions: context.keptActions ?? [],
      }) ? (
        <Text size="xs" c="dimmed">
          {t(($) => $.segments.eventsKept, { count: keptDays })}
        </Text>
      ) : null}
      {FilterEditor !== undefined && context.workspaceId !== undefined ? (
        <Modal
          opened={opened}
          onClose={close}
          size="xl"
          title={t(($) => $.segments.eventFilterTitle)}
        >
          <Stack>
            <FilterEditor
              workspaceId={context.workspaceId}
              action={action}
              initialValue={toCondition(count.filter ?? emptyGroup, 'eventFilter') ?? emptyGroup}
              onChange={(filter) => change(count.times, count.withinDays, filter)}
            />
            <Group justify="flex-end">
              <Button onClick={close}>{t(($) => $.segments.eventFilterDone)}</Button>
            </Group>
          </Stack>
        </Modal>
      ) : null}
    </Stack>
  )
}
