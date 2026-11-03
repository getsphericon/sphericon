import { Alert, Button, Group, Stack, TextInput } from '@mantine/core'
import type { ReactNode } from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

// TypedConfirmation is the additional confirmation of docs/standards/ui.md: it states the
// consequence and keeps the confirm button disabled until the exact name is typed. Workspace
// deletion (DeleteWorkspaceControl) and useConfirmation's `typedName` both render it, inside
// their own modal.
export function TypedConfirmation({
  warning,
  inputLabel,
  name,
  confirmLabel,
  loading = false,
  onCancel,
  onConfirm,
}: {
  warning: ReactNode
  inputLabel: ReactNode
  name: string
  confirmLabel: ReactNode
  loading?: boolean
  onCancel: () => void
  onConfirm: (typed: string) => void
}) {
  const { t } = useTranslation()
  const [typed, setTyped] = useState('')

  return (
    <Stack>
      <Alert color="red" variant="light">
        {warning}
      </Alert>
      <TextInput
        label={inputLabel}
        value={typed}
        onChange={(event) => setTyped(event.currentTarget.value)}
        data-autofocus
      />
      <Group justify="flex-end">
        <Button variant="default" onClick={onCancel}>
          {t(($) => $.actions.cancel)}
        </Button>
        <Button
          color="red"
          disabled={typed !== name}
          loading={loading}
          onClick={() => onConfirm(typed)}
        >
          {confirmLabel}
        </Button>
      </Group>
    </Stack>
  )
}
