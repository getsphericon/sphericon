import { Button, Group, Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteTemplatesDeleteMutation,
  siteTemplatesListOptions,
  siteTemplatesListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { templatesCreateRoute, templatesEditRoute, templatesRoute } from '../../router.tsx'

export function TemplatesListPage() {
  const { t } = useTranslation()
  const { slug } = templatesRoute.useParams()

  const deleteTemplate = useDeleteResource({
    mutation: siteTemplatesDeleteMutation(),
    slug,
    listQueryKey: siteTemplatesListQueryKey({ path: { slug } }),
    successMessage: t(($) => $.notifications.templateDeleted),
    errorTitle: t(($) => $.alerts.templateDeleteErrorTitle),
  })

  return (
    <Stack>
      <Group justify="flex-end">
        <ButtonLink to={templatesCreateRoute.to} params={{ slug }}>
          {t(($) => $.templates.addTemplate)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteTemplatesListOptions({ path: { slug }, query })}
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
              <Group gap="xs">
                <ButtonLink
                  size="compact-sm"
                  to={templatesEditRoute.to}
                  params={{ slug, templateId: record.id }}
                >
                  {t(($) => $.actions.edit)}
                </ButtonLink>
                <Button
                  size="compact-sm"
                  color="red"
                  variant="light"
                  onClick={() => deleteTemplate(record.id)}
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
