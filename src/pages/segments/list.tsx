import { Group, Menu, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteSegmentsArchiveMutation,
  siteSegmentsListOptions,
  siteSegmentsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { Route as segmentsDetailRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/index.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as segmentsCreateRoute } from '../../routes/workspaces/$workspaceId/segments/new.tsx'
import { withPage } from '../../searchSchemas.ts'

export function SegmentsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = segmentsRoute.useParams()
  const navigate = useNavigate()

  const archiveSegment = useResourceMutation({
    mutation: siteSegmentsArchiveMutation(),
    successMessage: t(($) => $.notifications.segmentArchived),
    errorTitle: t(($) => $.alerts.segmentArchiveErrorTitle),
  })
  const restoreSegment = useResourceMutation({
    mutation: siteSegmentsUnarchiveMutation(),
    successMessage: t(($) => $.notifications.segmentRestored),
    errorTitle: t(($) => $.alerts.segmentRestoreErrorTitle),
  })
  const { archived = false, page } = segmentsRoute.useSearch()
  const routeNavigate = segmentsRoute.useNavigate()

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.segments)}</Title>
        <ButtonLink to={segmentsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.segments.addSegment)}
        </ButtonLink>
      </Group>

      <ArchiveViewControl
        archived={archived}
        onChange={(next) =>
          void routeNavigate({
            search: { archived: next || undefined },
            replace: true,
            viewTransition: false,
          })
        }
      />

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void routeNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) =>
          siteSegmentsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.segmentLoadErrorTitle)}
        noRecordsText={t(($) => $.segments.noRecords)}
        emptyState={t(($) => $.segments.emptyState)}
        onRowClick={(record) =>
          void navigate({
            to: segmentsDetailRoute.to,
            params: { workspaceId, segmentId: record.id },
          })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.segments.nameLabel) },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <MenuItemLink
                    to={segmentsDetailRoute.to}
                    params={{ workspaceId, segmentId: record.id }}
                  >
                    {t(($) => $.actions.view)}
                  </MenuItemLink>
                  <MenuItemLink
                    to={segmentsEditRoute.to}
                    params={{ workspaceId, segmentId: record.id }}
                  >
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                  {record.archivedAt ? (
                    <Menu.Item
                      onClick={() =>
                        restoreSegment.mutate({ path: { workspaceId, id: record.id } })
                      }
                    >
                      {t(($) => $.actions.unarchive)}
                    </Menu.Item>
                  ) : (
                    <Menu.Item
                      color="red"
                      onClick={() =>
                        archiveSegment.mutate({ path: { workspaceId, id: record.id } })
                      }
                    >
                      {t(($) => $.actions.archive)}
                    </Menu.Item>
                  )}
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
