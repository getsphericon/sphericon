import { ActionIcon, Autocomplete, Button, Group, Input, Paper, Stack } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useMount } from 'ahooks'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EventActionOption } from '../../components/EventActionName.tsx'
import { useEventActionFilter } from '../../hooks/useEventActionLabel.ts'
import { endingsOf, endingsText, goalOf, goalText, withoutHalfTypedFilter } from './definition.ts'
import type { EndingDraft } from './definition.ts'
import { EventFilterField } from './EventFilterField.tsx'
import { TRIGGER_EVENTS } from './nodes.ts'

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

// The exit events end an active Enrollment early (#751), each an Event matcher (ADR 0133). The node
// holds them as the JSON text of their drafts; a filter left half-typed is dropped when the panel
// opens again, and one still being typed is refused at the Save.
export function ExitEventsControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const drafts = endingsOf(typeof data === 'string' ? data : '')
  // Each row keeps its key while the panel is open, so removing one never hands its builder to
  // the next.
  const [keys, setKeys] = useState(() => drafts.map(() => crypto.randomUUID()))
  const write = (next: EndingDraft[]) => props.handleChange(path, endingsText(next))
  useMount(() => {
    const loaded = drafts.map((draft) => withoutHalfTypedFilter(draft))
    if (loaded.some((draft, i) => draft !== drafts[i])) {
      write(loaded)
    }
  })

  return (
    <Input.Wrapper label={label} description={t(($) => $.automations.exitEventsHint)}>
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
          {t(($) => $.automations.exitEventAdd)}
        </Button>
      </Stack>
    </Input.Wrapper>
  )
}

// The goal event ends an active Enrollment as having reached its goal, an Event matcher (ADR 0133);
// none until one is added. The node holds it as the JSON text of its draft, '' for none.
export function GoalEventControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const draft = goalOf(typeof data === 'string' ? data : '')
  const write = (next: EndingDraft | null) => props.handleChange(path, goalText(next))
  useMount(() => {
    if (draft !== null && withoutHalfTypedFilter(draft) !== draft) {
      write(withoutHalfTypedFilter(draft))
    }
  })

  return (
    <Input.Wrapper label={label} description={t(($) => $.automations.goalEventHint)}>
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
