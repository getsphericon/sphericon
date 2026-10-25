import { Button, Group, Stack, Text } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesLeaveMutation,
  siteWorkspacesListQueryKey,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useConfirmation } from '../../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { Route as indexRoute } from '../../../routes/index.tsx'

// LeaveWorkspaceSection removes the caller's own Membership (Leaving, ADR 0050). Every Role
// can leave; the server refuses when the caller is the only Owner and says why.
export function LeaveWorkspaceSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const confirmLeave = useConfirmation()

  const leaveMutation = useResourceMutation({
    mutation: siteWorkspacesLeaveMutation(),
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.settings.danger.left),
    errorTitle: t(($) => $.settings.danger.leaveError),
    // The index route sends the User to another Workspace, or to the create screen.
    onDone: async () => navigate({ to: indexRoute.to }),
  })

  const onLeave = () =>
    confirmLeave({
      title: t(($) => $.settings.danger.leaveConfirmTitle),
      description: t(($) => $.settings.danger.leaveConfirmDescription),
      confirmLabel: t(($) => $.settings.danger.leaveConfirm),
      onConfirm: () => leaveMutation.mutate({ path: { workspaceId: workspace.id } }),
    })

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.danger.leaveDescription)}
      </Text>
      <Group>
        <Button variant="default" onClick={onLeave}>
          {t(($) => $.settings.danger.leaveButton)}
        </Button>
      </Group>
    </Stack>
  )
}
