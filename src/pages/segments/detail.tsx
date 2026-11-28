import { Avatar, Button, Group, Stack, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery, useSuspenseInfiniteQuery, useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { CenteredEmptyState } from '../../components/CenteredEmptyState.tsx'
import { ContactCsvExportModal } from '../../components/ContactCsvExportModal.tsx'
import { LoadMore } from '../../components/LoadMore.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton, TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSegmentContactsListInfiniteOptions,
  siteSegmentContactsListQueryKey,
  siteSegmentsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteContactResource } from '../../generated/site/types.gen.ts'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { Route as segmentsDetailRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/index.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { contactName } from '../../utils/contactName.ts'
import { can } from '../../utils/permissions.ts'
import { suspenseInfinite } from '../../utils/suspenseInfinite.ts'
import type { FieldColumn } from '../../utils/tableColumns.ts'
import { workspaceOptions } from '../../utils/workspace.ts'

const PAGE_SIZE = 25

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
  const segmentQuery = useSuspenseQuery(
    siteSegmentsGetOptions({ path: { workspaceId, id: segmentId } }),
  )
  const { data: workspace } = useQuery(workspaceOptions(workspaceId))
  const [exportOpened, { open: openExport, close: closeExport }] = useDisclosure(false)

  return (
    // It grows into the page's free height, so an empty list's state sits in the page's middle.
    <Stack flex={1}>
      <ContactCsvExportModal
        workspaceId={workspaceId}
        segmentId={segmentId}
        opened={exportOpened}
        onClose={closeExport}
      />
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Title order={4}>{segmentQuery.data.name}</Title>
        <Group gap="xs">
          {/* The export of the Segment's Contacts is offered only with contacts:export. */}
          {can(workspace, 'contacts:export') ? (
            <Button variant="default" onClick={openExport}>
              {t(($) => $.contactCsvExport.action)}
            </Button>
          ) : null}
          {segmentQuery.data.archivedAt ? null : (
            <ButtonLink
              variant="default"
              to={segmentsEditRoute.to}
              params={{ workspaceId, segmentId }}
            >
              {t(($) => $.actions.edit)}
            </ButtonLink>
          )}
          <ButtonLink variant="subtle" to={segmentsRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
        </Group>
      </Group>

      <Text fw={600}>{t(($) => $.segments.matchingContacts)}</Text>
      <QueryBoundary fallback={<TableSkeleton />} errorTitle={t(($) => $.alerts.loadErrorTitle)}>
        <SegmentContacts workspaceId={workspaceId} segmentId={segmentId} />
      </QueryBoundary>
    </Stack>
  )
}

// The Contacts the Segment matches, read a page at a time by cursor: the first under the page's
// boundary, each next one beneath the rows already shown.
function SegmentContacts({ workspaceId, segmentId }: { workspaceId: string; segmentId: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const options = { path: { workspaceId, segmentId }, query: { limit: PAGE_SIZE } }
  const query = useSuspenseInfiniteQuery({
    ...suspenseInfinite(
      siteSegmentContactsListInfiniteOptions(options),
      siteSegmentContactsListQueryKey(options),
    ),
    // The empty cursor is the first Contact.
    initialPageParam: '',
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  })
  const records = query.data.pages.flatMap((page) => page.items)

  if (records.length === 0) {
    return <CenteredEmptyState title={t(($) => $.segments.noMatchingContacts)} />
  }

  return (
    <Stack>
      <DataTable<SiteContactResource>
        withTableBorder
        // Striped, so the eye keeps its row across the columns (docs/standards/ui.md).
        striped
        highlightOnHover
        onRowClick={({ record }) =>
          void navigate({
            to: contactsDetailRoute.to,
            params: { workspaceId, contactId: record.id },
          })
        }
        records={records}
        columns={[
          {
            accessor: 'email',
            title: t(($) => $.table.email),
            render: (record) => (
              <Group gap="sm" wrap="nowrap">
                <Avatar
                  size="sm"
                  color="initials"
                  name={contactName(record)}
                  src={record.avatar ?? null}
                  alt={contactName(record)}
                />
                <Text size="sm">{record.email}</Text>
              </Group>
            ),
          },
          {
            accessor: 'firstName',
            title: t(($) => $.table.firstName),
          } satisfies FieldColumn<SiteContactResource>,
          {
            accessor: 'lastName',
            title: t(($) => $.table.lastName),
          } satisfies FieldColumn<SiteContactResource>,
        ]}
      />
      <LoadMore
        query={query}
        label={t(($) => $.segments.loadMoreContacts)}
        errorTitle={t(($) => $.alerts.loadErrorTitle)}
      />
    </Stack>
  )
}
