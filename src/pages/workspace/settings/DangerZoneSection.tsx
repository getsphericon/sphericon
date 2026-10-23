import { Stack, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { DeleteWorkspaceSection } from './DeleteWorkspaceSection.tsx'
import { LeaveWorkspaceSection } from './LeaveWorkspaceSection.tsx'

// DangerZoneSection holds what a User does to the Workspace as a whole: anyone leaves it,
// only an Owner deletes it.
export function DangerZoneSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()

  return (
    <Stack maw={420}>
      <Title order={3}>{t(($) => $.settings.danger.title)}</Title>
      <LeaveWorkspaceSection workspace={workspace} />
      <DeleteWorkspaceSection workspace={workspace} />
    </Stack>
  )
}
