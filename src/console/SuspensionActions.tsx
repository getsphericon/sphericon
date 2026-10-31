import { Alert, Button, Group, Modal, Stack, Textarea } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  staffWorkspacesExemptMutation,
  staffWorkspacesSuspendMutation,
  staffWorkspacesUnexemptMutation,
  staffWorkspacesUnsuspendMutation,
} from '../generated/staffee/@tanstack/react-query.gen.ts'
import { zStaffSuspendInput } from '../generated/staffee/zod.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { trimmed, useSchemaForm } from '../resources/schemaForm.ts'
import { useChangeNotice } from './useChangeNotice.ts'

const suspendSchema = z.object({ reason: trimmed(zStaffSuspendInput.shape.reason) })

function SuspendForm({ workspaceId, onClose }: { workspaceId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const form = useSchemaForm(suspendSchema, { reason: '' })
  const notifyUnchanged = useChangeNotice(
    t(($) => $.console.workspace.suspend.done),
    t(($) => $.console.workspace.suspend.unchanged),
  )
  const mutation = useResourceMutation({
    mutation: staffWorkspacesSuspendMutation(),
    errorTitle: t(($) => $.console.workspace.suspend.error),
    onDone: (change) => {
      notifyUnchanged(change)
      onClose()
    },
  })

  return (
    <form onSubmit={form.onSubmit((body) => mutation.mutate({ path: { workspaceId }, body }))}>
      <Stack>
        <Alert color="red" variant="light">
          {t(($) => $.console.workspace.suspend.warning)}
        </Alert>
        <Textarea
          label={t(($) => $.console.workspace.suspend.reasonLabel)}
          description={t(($) => $.console.workspace.suspend.reasonDescription)}
          autosize
          minRows={3}
          withAsterisk
          data-autofocus
          {...form.getInputProps('reason')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.console.workspace.suspend.cancel)}
          </Button>
          <Button type="submit" color="red" loading={mutation.isPending}>
            {t(($) => $.console.workspace.suspend.confirm)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// Suspend (reason form, then a red confirm button) or unsuspend (confirmation) one
// Workspace's outbound sending (ADR 0007). Login, the dashboard, API reads and /collect
// are unaffected, so the copy says only that sending stops.
export function SuspensionActions({
  workspaceId,
  suspended,
}: {
  workspaceId: string
  suspended: boolean
}) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const notifyUnchanged = useChangeNotice(
    t(($) => $.console.workspace.unsuspend.done),
    t(($) => $.console.workspace.unsuspend.unchanged),
  )
  const unsuspend = useResourceMutation({
    mutation: staffWorkspacesUnsuspendMutation(),
    errorTitle: t(($) => $.console.workspace.unsuspend.error),
    onDone: notifyUnchanged,
  })

  if (suspended) {
    return (
      <Button
        variant="light"
        loading={unsuspend.isPending}
        onClick={() =>
          void modals.openConfirmModal({
            title: t(($) => $.console.workspace.unsuspend.title),
            children: t(($) => $.console.workspace.unsuspend.description),
            labels: {
              cancel: t(($) => $.console.workspace.suspend.cancel),
              confirm: t(($) => $.console.workspace.unsuspend.confirm),
            },
            onConfirm: () => unsuspend.mutate({ path: { workspaceId } }),
          })
        }
      >
        {t(($) => $.console.workspace.unsuspend.button)}
      </Button>
    )
  }

  return (
    <>
      <Button color="red" variant="light" onClick={open}>
        {t(($) => $.console.workspace.suspend.button)}
      </Button>
      <Modal opened={opened} onClose={close} title={t(($) => $.console.workspace.suspend.title)}>
        <SuspendForm workspaceId={workspaceId} onClose={close} />
      </Modal>
    </>
  )
}

// Exempt a Workspace from the abuse detector, or police it again (ADR 0026). An exempt
// Workspace is neither warned nor frozen automatically but can still be suspended by hand.
export function ExemptionAction({ workspaceId, exempt }: { workspaceId: string; exempt: boolean }) {
  const { t } = useTranslation()
  const notifyUnchanged = useChangeNotice(
    exempt
      ? t(($) => $.console.workspace.abuse.unexemptDone)
      : t(($) => $.console.workspace.abuse.exemptDone),
    t(($) => $.console.workspace.abuse.unchanged),
  )
  const exemptMutation = useResourceMutation({
    mutation: staffWorkspacesExemptMutation(),
    errorTitle: t(($) => $.console.workspace.abuse.error),
    onDone: notifyUnchanged,
  })
  const unexemptMutation = useResourceMutation({
    mutation: staffWorkspacesUnexemptMutation(),
    errorTitle: t(($) => $.console.workspace.abuse.error),
    onDone: notifyUnchanged,
  })
  const mutation = exempt ? unexemptMutation : exemptMutation

  return (
    <Button
      variant="light"
      loading={mutation.isPending}
      onClick={() =>
        void modals.openConfirmModal({
          title: exempt
            ? t(($) => $.console.workspace.abuse.unexemptTitle)
            : t(($) => $.console.workspace.abuse.exemptTitle),
          children: exempt
            ? t(($) => $.console.workspace.abuse.unexemptDescription)
            : t(($) => $.console.workspace.abuse.exemptDescription),
          labels: {
            cancel: t(($) => $.console.workspace.suspend.cancel),
            confirm: exempt
              ? t(($) => $.console.workspace.abuse.unexemptButton)
              : t(($) => $.console.workspace.abuse.exemptButton),
          },
          onConfirm: () => mutation.mutate({ path: { workspaceId } }),
        })
      }
    >
      {exempt
        ? t(($) => $.console.workspace.abuse.unexemptButton)
        : t(($) => $.console.workspace.abuse.exemptButton)}
    </Button>
  )
}
