import { Button, Group, Stack, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsCreateMutation,
  siteAutomationsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as automationsCreateRoute } from '../../routes/workspaces/$workspaceId/automations/new.tsx'
import { AutomationDetailsFields } from './AutomationDetailsFields.tsx'
import type { AutomationDetailsValues } from './AutomationDetailsFields.tsx'

// Create captures only name + trigger, then redirects into the visual builder
// (edit) where the flow itself is designed — so the builder always has an id.
export function AutomationCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId } = automationsCreateRoute.useParams()

  const form = useForm<AutomationDetailsValues>({
    initialValues: { name: '', triggerEvent: 'contact.created' },
  })

  const createMutation = useResourceMutation({
    mutation: siteAutomationsCreateMutation(),
    invalidate: [siteAutomationsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.notifications.automationCreated),
    errorTitle: t(($) => $.alerts.automationSaveErrorTitle),
    onDone: (created) =>
      navigate({ to: automationsEditRoute.to, params: { workspaceId, automationId: created.id } }),
  })

  return (
    <Stack>
      <Title order={4}>{t(($) => $.automations.createTitle)}</Title>
      <form
        onSubmit={form.onSubmit((values) =>
          createMutation.mutate({
            path: { workspaceId },
            body: { name: values.name.trim(), triggerEvent: values.triggerEvent },
          }),
        )}
      >
        <Stack>
          <AutomationDetailsFields form={form} />
          <Group justify="flex-end">
            <ButtonLink variant="default" to={automationsRoute.to} params={{ workspaceId }}>
              {t(($) => $.actions.cancel)}
            </ButtonLink>
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.actions.continue)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Stack>
  )
}
