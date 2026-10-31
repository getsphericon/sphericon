import { Button, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { UserMenu } from '../../components/UserMenu.tsx'
import {
  siteWorkspacesCreateMutation,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'

// CreateWorkspacePage is the one create flow: reached from the workspace switcher, and the
// page a User with no Workspace lands on. A name is all it asks (ADR 0043).
export function CreateWorkspacePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const workspaces = useQuery(siteWorkspacesListOptions())
  const hasWorkspaces = (workspaces.data?.length ?? 0) > 0
  const form = useForm({ initialValues: { name: '' } })

  const createMutation = useResourceMutation({
    mutation: siteWorkspacesCreateMutation(),
    errorTitle: t(($) => $.createWorkspace.errorTitle),
    forbiddenMessage: t(($) => $.createWorkspace.verifyFirst),
    onDone: async (created) =>
      navigate({ to: overviewRoute.to, params: { workspaceId: created.id } }),
  })

  return (
    <CenteredLayout maw={420}>
      <Group justify="flex-end">
        <UserMenu />
      </Group>
      <Title order={3}>{t(($) => $.createWorkspace.title)}</Title>
      <Text c="dimmed">
        {hasWorkspaces
          ? t(($) => $.createWorkspace.descriptionAnother)
          : t(($) => $.createWorkspace.descriptionFirst)}
      </Text>
      <form
        onSubmit={form.onSubmit((values) =>
          createMutation.mutate({ body: { name: values.name.trim() } }),
        )}
      >
        <Stack>
          <TextInput
            label={t(($) => $.createWorkspace.nameLabel)}
            description={t(($) => $.createWorkspace.nameDescription)}
            required
            data-autofocus
            {...form.getInputProps('name')}
          />
          <Group justify="flex-end">
            {hasWorkspaces ? (
              <ButtonLink variant="default" to={indexRoute.to}>
                {t(($) => $.actions.cancel)}
              </ButtonLink>
            ) : null}
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.createWorkspace.submit)}
            </Button>
          </Group>
        </Stack>
      </form>
    </CenteredLayout>
  )
}
