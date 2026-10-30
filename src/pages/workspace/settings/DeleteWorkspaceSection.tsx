import { Stack, Text } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DeleteWorkspaceControl } from '../../../components/DeleteWorkspaceControl.tsx'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { Route as indexRoute } from '../../../routes/index.tsx'
import { can } from '../../../utils/permissions.ts'

// DeleteWorkspaceSection is the delete part of the danger zone (ADR 0043): an Owner
// deletes the Workspace by typing its name. Deletion is soft, so the copy says how long
// it can be restored.
export function DeleteWorkspaceSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  if (!can(workspace, 'workspace:delete')) {
    return null
  }

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.danger.description)}
      </Text>
      <DeleteWorkspaceControl
        workspace={workspace}
        label={t(($) => $.settings.danger.deleteButton)}
        // The index route sends the User to another Workspace, or to the create screen.
        onDone={async () => navigate({ to: indexRoute.to })}
      />
    </Stack>
  )
}
