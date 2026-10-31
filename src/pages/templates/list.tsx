import { Group, Menu, Stack, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteTemplatesDeleteMutation,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$workspaceId/templates/$templateId/edit.tsx'
import { Route as templatesRoute } from '../../routes/workspaces/$workspaceId/templates/index.tsx'
import { Route as templatesCreateRoute } from '../../routes/workspaces/$workspaceId/templates/new.tsx'
import { withPage } from '../../searchSchemas.ts'

export function TemplatesListPage() {
  const { page } = templatesRoute.useSearch()
  const pageNavigate = templatesRoute.useNavigate()
  const { t } = useTranslation()
  const { workspaceId } = templatesRoute.useParams()

  const deleteTemplate = useDeleteResource({
    mutation: siteTemplatesDeleteMutation(),
    workspaceId,
    successMessage: t(($) => $.notifications.templateDeleted),
    errorTitle: t(($) => $.alerts.templateDeleteErrorTitle),
  })

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.templates)}</Title>
        <ButtonLink to={templatesCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.templates.addTemplate)}
        </ButtonLink>
      </Group>

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void pageNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) => siteTemplatesListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.templateLoadErrorTitle)}
        noRecordsText={t(($) => $.templates.noRecords)}
        emptyState={t(($) => $.templates.emptyState)}
        columns={[
          { accessor: 'name', title: t(($) => $.templates.nameLabel) },
          { accessor: 'subject', title: t(($) => $.templates.subjectLabel) },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <MenuItemLink
                    to={templatesEditRoute.to}
                    params={{ workspaceId, templateId: record.id }}
                  >
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                  <Menu.Item color="red" onClick={() => deleteTemplate(record.id)}>
                    {t(($) => $.actions.delete)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
