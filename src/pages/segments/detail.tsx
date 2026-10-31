import { Group, Stack, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSegmentContactsListOptions,
  siteSegmentsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { Route as segmentsDetailRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/index.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'

export function SegmentDetailPage() {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={<PageSkeleton />}
      errorTitle={t(($) => $.alerts.segmentLoadErrorTitle)}
    >
      <SegmentDetail />
    </QueryBoundary>
  )
}

function SegmentDetail() {
  const { t } = useTranslation()
  const { workspaceId, segmentId } = segmentsDetailRoute.useParams()
  const navigate = useNavigate()
  const segmentQuery = useSuspenseQuery(
    siteSegmentsGetOptions({ path: { workspaceId, id: segmentId } }),
  )

  return (
    <Stack>
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Title order={4}>{segmentQuery.data.name}</Title>
        <Group gap="xs">
          <ButtonLink
            variant="default"
            to={segmentsEditRoute.to}
            params={{ workspaceId, segmentId }}
          >
            {t(($) => $.actions.edit)}
          </ButtonLink>
          <ButtonLink variant="subtle" to={segmentsRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
        </Group>
      </Group>

      <Title order={5}>{t(($) => $.segments.matchingContacts)}</Title>
      <PagedTable
        options={(query) =>
          siteSegmentContactsListOptions({ path: { workspaceId, segmentId }, query })
        }
        loadErrorTitle={t(($) => $.alerts.loadErrorTitle)}
        noRecordsText={t(($) => $.segments.noMatchingContacts)}
        columns={[
          { accessor: 'email', title: t(($) => $.table.email) },
          { accessor: 'firstName', title: t(($) => $.table.firstName) },
          { accessor: 'lastName', title: t(($) => $.table.lastName) },
        ]}
        onRowClick={(record) =>
          void navigate({
            to: contactsDetailRoute.to,
            params: { workspaceId, contactId: record.id },
          })
        }
      />
    </Stack>
  )
}
