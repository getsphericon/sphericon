import { Button, Modal, Stack } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useTranslation } from 'react-i18next'

import { siteWorkspacesDeleteMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { TypedConfirmation } from './TypedConfirmation.tsx'

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

  const deleteMutation = useResourceMutation({
    mutation: siteWorkspacesDeleteMutation(),
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
        <TypedConfirmation
          warning={t(($) => $.settings.danger.warning)}
          inputLabel={t(($) => $.settings.danger.confirmLabel, { name: workspace.name })}
          name={workspace.name}
          confirmLabel={t(($) => $.settings.danger.confirmButton)}
          loading={deleteMutation.isPending}
          onCancel={close}
          onConfirm={(typed) =>
            deleteMutation.mutate({ path: { workspaceId: workspace.id }, body: { name: typed } })
          }
        />
      </Modal>
    </Stack>
  )
}
