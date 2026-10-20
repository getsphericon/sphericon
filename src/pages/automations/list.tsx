import { Badge, Button, Group, Stack, Switch, Title } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsActivateMutation,
  siteAutomationsArchiveMutation,
  siteAutomationsDeactivateMutation,
  siteAutomationsDeleteMutation,
  siteAutomationsListOptions,
  siteAutomationsListQueryKey,
  siteAutomationsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as automationsCreateRoute } from '../../routes/workspaces/$workspaceId/automations/new.tsx'

export function AutomationsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = automationsRoute.useParams()

  const listQueryKey = siteAutomationsListQueryKey({ path: { workspaceId } })

  const deleteAutomation = useDeleteResource({
    mutation: siteAutomationsDeleteMutation(),
    workspaceId,
    listQueryKey,
    successMessage: t(($) => $.notifications.automationDeleted),
    errorTitle: t(($) => $.alerts.automationDeleteErrorTitle),
  })

  const activateMutation = useResourceMutation({
    mutation: siteAutomationsActivateMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationActivated),
    errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
  })

  const deactivateMutation = useResourceMutation({
    mutation: siteAutomationsDeactivateMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationDeactivated),
    errorTitle: t(($) => $.alerts.automationStatusErrorTitle),
  })

  const archiveMutation = useResourceMutation({
    mutation: siteAutomationsArchiveMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationArchived),
    errorTitle: t(($) => $.alerts.automationArchiveErrorTitle),
  })

  const restoreMutation = useResourceMutation({
    mutation: siteAutomationsUnarchiveMutation(),
    invalidate: [listQueryKey],
    successMessage: t(($) => $.notifications.automationRestored),
    errorTitle: t(($) => $.alerts.automationRestoreErrorTitle),
  })

  const [archived, setArchived] = useState(false)

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.automations)}</Title>
        <ButtonLink to={automationsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.automations.addAutomation)}
        </ButtonLink>
      </Group>

      <Switch
        label={t(($) => $.automations.showArchived)}
        checked={archived}
        onChange={(event) => setArchived(event.currentTarget.checked)}
      />

      <PagedTable
        key={String(archived)}
        options={(query) =>
          siteAutomationsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.automationLoadErrorTitle)}
        noRecordsText={t(($) => $.automations.noRecords)}
        emptyState={t(($) => $.automations.emptyState)}
        columns={[
          { accessor: 'name', title: t(($) => $.automations.nameLabel) },
          { accessor: 'triggerEvent', title: t(($) => $.automations.triggerColumn) },
          {
            accessor: 'status',
            title: t(($) => $.automations.statusLabel),
            render: (record) => (
              <Badge variant="light" color={record.status === 'active' ? 'teal' : 'gray'}>
                {record.status === 'active'
                  ? t(($) => $.automations.statusActive)
                  : t(($) => $.automations.statusDraft)}
              </Badge>
            ),
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group gap="xs">
                {record.archivedAt ? (
                  <Button
                    size="compact-sm"
                    variant="light"
                    loading={restoreMutation.isPending}
                    onClick={() => restoreMutation.mutate({ path: { workspaceId, id: record.id } })}
                  >
                    {t(($) => $.actions.unarchive)}
                  </Button>
                ) : (
                  <>
                    {record.status === 'active' ? (
                      <Button
                        size="compact-sm"
                        variant="light"
                        color="orange"
                        loading={deactivateMutation.isPending}
                        onClick={() =>
                          deactivateMutation.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.automations.deactivate)}
                      </Button>
                    ) : (
                      <Button
                        size="compact-sm"
                        variant="light"
                        color="teal"
                        loading={activateMutation.isPending}
                        onClick={() =>
                          activateMutation.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.automations.activate)}
                      </Button>
                    )}
                    <ButtonLink
                      size="compact-sm"
                      to={automationsEditRoute.to}
                      params={{ workspaceId, automationId: record.id }}
                    >
                      {t(($) => $.actions.edit)}
                    </ButtonLink>
                    <Button
                      size="compact-sm"
                      color="red"
                      variant="light"
                      loading={archiveMutation.isPending}
                      onClick={() =>
                        archiveMutation.mutate({ path: { workspaceId, id: record.id } })
                      }
                    >
                      {t(($) => $.actions.archive)}
                    </Button>
                    <Button
                      size="compact-sm"
                      color="red"
                      variant="light"
                      onClick={() => deleteAutomation(record.id)}
                    >
                      {t(($) => $.actions.delete)}
                    </Button>
                  </>
                )}
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
