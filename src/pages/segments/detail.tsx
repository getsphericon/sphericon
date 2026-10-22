import { Group, Loader, Stack, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
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
  const { workspaceId, segmentId } = segmentsDetailRoute.useParams()
  const navigate = useNavigate()
  const segmentQuery = useQuery(siteSegmentsGetOptions({ path: { workspaceId, id: segmentId } }))

  if (segmentQuery.isLoading) {
    return <Loader />
  }

  if (segmentQuery.isError || !segmentQuery.data) {
    return (
      <ApiErrorAlert
        error={segmentQuery.error}
        title={t(($) => $.alerts.segmentLoadErrorTitle)}
        fallback={t(($) => $.alerts.segmentLoadErrorTitle)}
      />
    )
  }

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
