import { Alert, Button, Group, Modal, Stack, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesDeleteMutation,
  siteWorkspacesListDeletedQueryKey,
  siteWorkspacesListQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'

// DeleteWorkspaceControl deletes a Workspace once its name is typed (ADR 0043): a button that
// opens the confirmation, which warns what the deletion freezes. The server checks the typed name
// again. onDone runs once the Workspace is deleted.
export function DeleteWorkspaceControl({
  workspace,
  label,
  onDone,
}: {
  workspace: { id: string; name: string }
  label: string
  onDone: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const [typed, setTyped] = useState('')

  const deleteMutation = useResourceMutation({
    mutation: siteWorkspacesDeleteMutation(),
    invalidate: [siteWorkspacesListQueryKey(), siteWorkspacesListDeletedQueryKey()],
    successMessage: t(($) => $.settings.danger.deleted),
    errorTitle: t(($) => $.settings.danger.errorTitle),
    onDone,
  })

  return (
    <Stack gap={0} align="flex-start">
      <Button color="red" variant="outline" onClick={open}>
        {label}
      </Button>
      <Modal opened={opened} onClose={close} title={t(($) => $.settings.danger.modalTitle)}>
        <Stack>
          <Alert color="red" variant="light">
            {t(($) => $.settings.danger.warning)}
          </Alert>
          <TextInput
            label={t(($) => $.settings.danger.confirmLabel, { name: workspace.name })}
            value={typed}
            onChange={(event) => setTyped(event.currentTarget.value)}
            data-autofocus
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={close}>
              {t(($) => $.actions.cancel)}
            </Button>
            <Button
              color="red"
              disabled={typed !== workspace.name}
              loading={deleteMutation.isPending}
              onClick={() =>
                deleteMutation.mutate({
                  path: { workspaceId: workspace.id },
                  body: { name: typed },
                })
              }
            >
              {t(($) => $.settings.danger.confirmButton)}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
