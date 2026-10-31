import { Button, Card, Group, Modal, Stack, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import type { UseMutationOptions } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { SendCapForm } from './SendCapForm.tsx'
import { useChangeNotice } from './useChangeNotice.ts'

// The words of one Send cap card, a Workspace's or the Instance's default, already translated.
interface SendCapCopy {
  title: string
  // The Send cap as it stands: its value, or that there is none.
  summary: string
  description: string
  set: string
  change: string
  remove: string
  removeTitle: string
  removeDescription: string
  formTitle: string
  formDescription: string
  saved: string
  removed: string
  unchanged: string
  error: string
}

// A change of a Send cap answers whether anything changed.
interface Change {
  changed: boolean
}

interface SendCapCardProps<
  TSave extends Change,
  TSaveError,
  TSaveVars,
  TSaveCtx,
  TClear extends Change,
  TClearError,
  TClearVars,
  TClearCtx,
> {
  perDay: number | null
  copy: SendCapCopy
  // What the card shows under the value, such as the sends of the last 24 hours.
  children?: ReactNode
  // The generated mutation that sets the Send cap, and its variables for the form's body.
  save: UseMutationOptions<TSave, TSaveError, TSaveVars, TSaveCtx>
  saveVariables: (body: { perDay: number }) => TSaveVars
  // The generated mutation that removes it, and its variables.
  clear: UseMutationOptions<TClear, TClearError, TClearVars, TClearCtx>
  clearVariables: TClearVars
}

// A Send cap (ADR 0067) a Staff member sets, changes or removes: the value, a form in a modal to set
// or change it, and a confirmed removal. Each change says whether anything changed.
export function SendCapCard<
  TSave extends Change,
  TSaveError,
  TSaveVars,
  TSaveCtx,
  TClear extends Change,
  TClearError,
  TClearVars,
  TClearCtx,
>({
  perDay,
  copy,
  children,
  save,
  saveVariables,
  clear,
  clearVariables,
}: SendCapCardProps<
  TSave,
  TSaveError,
  TSaveVars,
  TSaveCtx,
  TClear,
  TClearError,
  TClearVars,
  TClearCtx
>) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const notifyCleared = useChangeNotice(copy.removed, copy.unchanged)
  const clearing = useResourceMutation({
    mutation: clear,
    errorTitle: copy.error,
    onDone: notifyCleared,
  })

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        {copy.title}
      </Title>
      <Stack gap="xs">
        <Text size="sm">{copy.summary}</Text>
        {children}
        <Text size="sm" c="dimmed">
          {copy.description}
        </Text>
      </Stack>
      <Group mt="md">
        <Button variant="light" onClick={open}>
          {perDay === null ? copy.set : copy.change}
        </Button>
        {perDay === null ? null : (
          <Button
            variant="subtle"
            color="red"
            loading={clearing.isPending}
            onClick={() =>
              void modals.openConfirmModal({
                title: copy.removeTitle,
                children: copy.removeDescription,
                labels: { cancel: t(($) => $.actions.cancel), confirm: copy.remove },
                onConfirm: () => clearing.mutate(clearVariables),
              })
            }
          >
            {copy.remove}
          </Button>
        )}
      </Group>
      <Modal opened={opened} onClose={close} title={copy.formTitle}>
        <SaveForm
          perDay={perDay}
          copy={copy}
          save={save}
          saveVariables={saveVariables}
          onClose={close}
        />
      </Modal>
    </Card>
  )
}

function SaveForm<TSave extends Change, TSaveError, TSaveVars, TSaveCtx>({
  perDay,
  copy,
  save,
  saveVariables,
  onClose,
}: {
  perDay: number | null
  copy: SendCapCopy
  save: UseMutationOptions<TSave, TSaveError, TSaveVars, TSaveCtx>
  saveVariables: (body: { perDay: number }) => TSaveVars
  onClose: () => void
}) {
  const notify = useChangeNotice(copy.saved, copy.unchanged)
  const saving = useResourceMutation({
    mutation: save,
    errorTitle: copy.error,
    onDone: (change) => {
      notify(change)
      onClose()
    },
  })

  return (
    <SendCapForm
      perDay={perDay}
      description={copy.formDescription}
      pending={saving.isPending}
      onSubmit={(body) => saving.mutate(saveVariables(body))}
      onCancel={onClose}
    />
  )
}
