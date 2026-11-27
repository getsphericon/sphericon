import { ActionIcon, Autocomplete, Button, Group, Input, Paper, Stack } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import { useMount } from 'ahooks'
import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EventActionOption } from '../../components/EventActionName.tsx'
import { useEventActionFilter } from '../../hooks/useEventActionLabel.ts'
import { withoutHalfTypedFilter } from './definition.ts'
import type { EndingDraft } from './definition.ts'
import { EventFilterField } from './EventFilterField.tsx'
import { TRIGGER_EVENTS } from './triggerEvents.ts'

const NO_ENDING: EndingDraft = { action: '', filterText: '' }

// One exit or goal event (ADR 0133): its action, any Event action with the built-in events as
// suggestions, and its filter on the Event's own properties, the editor the Trigger's filter uses.
function EndingEditor({
  draft,
  onChange,
  onRemove,
}: {
  draft: EndingDraft
  onChange: (draft: EndingDraft) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()

  return (
    <Paper withBorder p="xs">
      <Stack gap="xs">
        <Group gap="xs" align="flex-end" wrap="nowrap">
          <Autocomplete
            flex={1}
            label={t(($) => $.automations.endingActionLabel)}
            data={[...TRIGGER_EVENTS]}
            renderOption={({ option }) => <EventActionOption action={option.value} />}
            filter={actionFilter}
            value={draft.action}
            onChange={(action) => onChange({ ...draft, action: action.trim() })}
          />
          <ActionIcon
            variant="subtle"
            color="red"
            size="lg"
            aria-label={t(($) => $.automations.endingRemove)}
            onClick={onRemove}
          >
            <IconTrash size={16} />
          </ActionIcon>
        </Group>
        <Input.Wrapper label={t(($) => $.automations.triggerFilterLabel)}>
          <EventFilterField
            action={draft.action}
            text={draft.filterText}
            onChange={(filterText) => onChange({ ...draft, filterText })}
          />
        </Input.Wrapper>
      </Stack>
    </Paper>
  )
}

// A list of Event matchers (ADR 0133), each its action and its filter, with a button that adds one:
// the exit events of the Trigger, or the Events a Wait for event awaits. A filter left half-typed is
// dropped when the panel opens again, and one still being typed is refused at the Save.
export function MatcherListControl({
  label,
  description,
  addLabel,
  error,
  value: drafts,
  onChange: write,
}: {
  label: string
  description: string
  addLabel: string
  error?: ReactNode
  value: EndingDraft[]
  onChange: (drafts: EndingDraft[]) => void
}) {
  // Each row keeps its key while the panel is open, so removing one never hands its builder to
  // the next.
  const [keys, setKeys] = useState(() => drafts.map(() => crypto.randomUUID()))
  useMount(() => {
    const loaded = drafts.map((draft) => withoutHalfTypedFilter(draft))
    if (loaded.some((draft, i) => draft !== drafts[i])) {
      write(loaded)
    }
  })

  return (
    <Input.Wrapper label={label} description={description} error={error}>
      <Stack gap="xs" mt="xs">
        {drafts.map((draft, i) => (
          <EndingEditor
            key={keys[i] ?? i}
            draft={draft}
            onChange={(next) => write(drafts.map((d, j) => (j === i ? next : d)))}
            onRemove={() => {
              setKeys(keys.filter((_, j) => j !== i))
              write(drafts.filter((_, j) => j !== i))
            }}
          />
        ))}
        <Button
          variant="light"
          leftSection={<IconPlus size={16} />}
          onClick={() => {
            setKeys([...keys, crypto.randomUUID()])
            write([...drafts, NO_ENDING])
          }}
        >
          {addLabel}
        </Button>
      </Stack>
    </Input.Wrapper>
  )
}

// The exit events end an active Enrollment early (#751), each an Event matcher (ADR 0133).
export function ExitEventsControl({
  value,
  onChange,
}: {
  value: EndingDraft[]
  onChange: (drafts: EndingDraft[]) => void
}) {
  const { t } = useTranslation()
  return (
    <MatcherListControl
      label={t(($) => $.automations.exitEventsLabel)}
      description={t(($) => $.automations.exitEventsHint)}
      addLabel={t(($) => $.automations.exitEventAdd)}
      value={value}
      onChange={onChange}
    />
  )
}

// The goal event ends an active Enrollment as having reached its goal, an Event matcher (ADR 0133);
// none until one is added.
export function GoalEventControl({
  value: draft,
  onChange: write,
}: {
  value: EndingDraft | null
  onChange: (draft: EndingDraft | null) => void
}) {
  const { t } = useTranslation()
  useMount(() => {
    if (draft !== null && withoutHalfTypedFilter(draft) !== draft) {
      write(withoutHalfTypedFilter(draft))
    }
  })

  return (
    <Input.Wrapper
      label={t(($) => $.automations.goalEventLabel)}
      description={t(($) => $.automations.goalEventHint)}
    >
      <Stack gap="xs" mt="xs">
        {draft === null ? (
          <Button
            variant="light"
            leftSection={<IconPlus size={16} />}
            onClick={() => write(NO_ENDING)}
          >
            {t(($) => $.automations.goalEventAdd)}
          </Button>
        ) : (
          <EndingEditor draft={draft} onChange={write} onRemove={() => write(null)} />
        )}
      </Stack>
    </Input.Wrapper>
  )
}
