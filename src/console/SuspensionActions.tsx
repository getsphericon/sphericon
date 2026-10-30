import { Alert, Button, Group, Modal, Stack, Textarea } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { useTranslation } from 'react-i18next'

import {
  staffWorkspacesGetQueryKey,
  staffWorkspacesListQueryKey,
  staffWorkspacesExemptMutation,
  staffWorkspacesSuspendMutation,
  staffWorkspacesUnexemptMutation,
  staffWorkspacesUnsuspendMutation,
} from '../generated/staffee/@tanstack/react-query.gen.ts'
import type { StaffExemptionChange, StaffSuspensionChange } from '../generated/staffee/types.gen.ts'
import { useNotify } from '../hooks/useNotify.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'

// Both actions answer 200 with whether anything changed, so a no-op says so instead of
// looking like a success.
function useChangeNotice(changedMessage: string, unchangedMessage: string) {
  const { t } = useTranslation()
  const notify = useNotify()
  return (change: StaffSuspensionChange | StaffExemptionChange) => {
    notify(
      change.changed
        ? {
            color: 'teal',
            title: t(($) => $.notifications.successTitle),
            message: changedMessage,
          }
        : {
            color: 'blue',
            title: t(($) => $.console.workspace.unchangedTitle),
            message: unchangedMessage,
          },
    )
  }
}

function useInvalidate(workspaceId: string) {
  return [staffWorkspacesGetQueryKey({ path: { workspaceId } }), staffWorkspacesListQueryKey()]
}

function SuspendForm({ workspaceId, onClose }: { workspaceId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const form = useForm({
    initialValues: { reason: '' },
    validate: {
      reason: (value) =>
        value.trim() ? null : t(($) => $.console.workspace.suspend.reasonRequired),
    },
  })
  const notifyUnchanged = useChangeNotice(
    t(($) => $.console.workspace.suspend.done),
    t(($) => $.console.workspace.suspend.unchanged),
  )
  const mutation = useResourceMutation({
    mutation: staffWorkspacesSuspendMutation(),
    invalidate: useInvalidate(workspaceId),
    errorTitle: t(($) => $.console.workspace.suspend.error),
    onDone: (change) => {
      notifyUnchanged(change)
      onClose()
    },
  })

  return (
    <form
      onSubmit={form.onSubmit(({ reason }) =>
        mutation.mutate({ path: { workspaceId }, body: { reason: reason.trim() } }),
      )}
    >
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
    invalidate: useInvalidate(workspaceId),
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
    invalidate: useInvalidate(workspaceId),
    errorTitle: t(($) => $.console.workspace.abuse.error),
    onDone: notifyUnchanged,
  })
  const unexemptMutation = useResourceMutation({
    mutation: staffWorkspacesUnexemptMutation(),
    invalidate: useInvalidate(workspaceId),
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
