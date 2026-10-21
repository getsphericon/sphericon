import { Loader, Stack } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { siteSetupChecklistGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { SetupChecklist } from './SetupChecklist.tsx'

// OverviewPage is the workspace's landing page: the Setup checklist until it is finished.
export function OverviewPage() {
  const { t } = useTranslation()
  const { workspaceId } = overviewRoute.useParams()

  const checklistQuery = useQuery(siteSetupChecklistGetOptions({ path: { workspaceId } }))
  const checklist = checklistQuery.data
  const showChecklist = checklist !== undefined && !checklist.complete

  return (
    <Stack>
      {checklistQuery.isLoading && <Loader />}

      {checklistQuery.isError && (
        <ApiErrorAlert
          error={checklistQuery.error}
          title={t(($) => $.setupChecklist.loadError)}
          fallback={t(($) => $.setupChecklist.loadError)}
        />
      )}

      {showChecklist && <SetupChecklist workspaceId={workspaceId} checklist={checklist} />}
    </Stack>
  )
}
