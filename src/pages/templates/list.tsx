import { Group, Menu, Stack, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteTemplatesDeleteMutation,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteEmailTemplateResource } from '../../generated/site/types.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$workspaceId/templates/$templateId/edit.tsx'
import { Route as templatesRoute } from '../../routes/workspaces/$workspaceId/templates/index.tsx'
import { Route as templatesCreateRoute } from '../../routes/workspaces/$workspaceId/templates/new.tsx'
import { withPage } from '../../searchSchemas.ts'
import type { FieldColumn } from '../../utils/tableColumns.ts'

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
    consequence: t(($) => $.confirmations.deleteTemplate),
  })

  return (
    <Stack flex={1}>
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
        collection={t(($) => $.nav.templates)}
        empty={{
          title: t(($) => $.templates.noRecords),
          action: (
            <ButtonLink variant="default" to={templatesCreateRoute.to} params={{ workspaceId }}>
              {t(($) => $.templates.addTemplate)}
            </ButtonLink>
          ),
        }}
        columns={[
          {
            accessor: 'name',
            title: t(($) => $.templates.nameLabel),
          } satisfies FieldColumn<SiteEmailTemplateResource>,
          {
            accessor: 'subject',
            title: t(($) => $.templates.subjectLabel),
          } satisfies FieldColumn<SiteEmailTemplateResource>,
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
