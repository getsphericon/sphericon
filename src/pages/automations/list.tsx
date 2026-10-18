import { Badge, Button, Group, Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsActivateMutation,
  siteAutomationsDeactivateMutation,
  siteAutomationsDeleteMutation,
  siteAutomationsListOptions,
  siteAutomationsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$slug/automations/$automationId/edit.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$slug/automations/index.tsx'
import { Route as automationsCreateRoute } from '../../routes/workspaces/$slug/automations/new.tsx'

export function AutomationsListPage() {
  const { t } = useTranslation()
  const { slug } = automationsRoute.useParams()

  const listQueryKey = siteAutomationsListQueryKey({ path: { slug } })

  const deleteAutomation = useDeleteResource({
    mutation: siteAutomationsDeleteMutation(),
    slug,
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

  return (
    <Stack>
      <Group justify="flex-end">
        <ButtonLink to={automationsCreateRoute.to} params={{ slug }}>
          {t(($) => $.automations.addAutomation)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteAutomationsListOptions({ path: { slug }, query })}
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
                {record.status === 'active' ? (
                  <Button
                    size="compact-sm"
                    variant="light"
                    color="orange"
                    loading={deactivateMutation.isPending}
                    onClick={() => deactivateMutation.mutate({ path: { slug, id: record.id } })}
                  >
                    {t(($) => $.automations.deactivate)}
                  </Button>
                ) : (
                  <Button
                    size="compact-sm"
                    variant="light"
                    color="teal"
                    loading={activateMutation.isPending}
                    onClick={() => activateMutation.mutate({ path: { slug, id: record.id } })}
                  >
                    {t(($) => $.automations.activate)}
                  </Button>
                )}
                <ButtonLink
                  size="compact-sm"
                  to={automationsEditRoute.to}
                  params={{ slug, automationId: record.id }}
                >
                  {t(($) => $.actions.edit)}
                </ButtonLink>
                <Button
                  size="compact-sm"
                  color="red"
                  variant="light"
                  onClick={() => deleteAutomation(record.id)}
                >
                  {t(($) => $.actions.delete)}
                </Button>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
