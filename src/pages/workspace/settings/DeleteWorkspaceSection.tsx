import { Alert, Button, Group, Modal, Stack, Text, TextInput, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesDeleteMutation,
  siteWorkspacesListDeletedQueryKey,
  siteWorkspacesListQueryKey,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { Route as indexRoute } from '../../../routes/index.tsx'

// DeleteWorkspaceSection is the danger section of the settings (ADR 0043): an Owner
// deletes the Workspace by typing its name. Deletion is soft, so the copy says how long
// it can be restored. The server checks the typed name again.
export function DeleteWorkspaceSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [opened, { open, close }] = useDisclosure(false)
  const [typed, setTyped] = useState('')

  const deleteMutation = useResourceMutation({
    mutation: siteWorkspacesDeleteMutation(),
    invalidate: [siteWorkspacesListQueryKey(), siteWorkspacesListDeletedQueryKey()],
    successMessage: t(($) => $.settings.danger.deleted),
    errorTitle: t(($) => $.settings.danger.errorTitle),
    // The index route sends the User to another Workspace, or to the create screen.
    onDone: () => navigate({ to: indexRoute.to }),
  })

  // Only an Owner deletes a Workspace.
  if (workspace.role !== 'owner') {
    return null
  }

  return (
    <Stack maw={420}>
      <Title order={3}>{t(($) => $.settings.danger.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.danger.description)}
      </Text>
      <Group>
        <Button color="red" variant="light" onClick={open}>
          {t(($) => $.settings.danger.deleteButton)}
        </Button>
      </Group>
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
