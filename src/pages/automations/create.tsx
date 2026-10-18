import { Button, Group, Stack, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteAutomationsCreateMutation,
  siteAutomationsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$slug/automations/$automationId/edit.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$slug/automations/index.tsx'
import { Route as automationsCreateRoute } from '../../routes/workspaces/$slug/automations/new.tsx'
import { AutomationDetailsFields } from './AutomationDetailsFields.tsx'
import type { AutomationDetailsValues } from './AutomationDetailsFields.tsx'

// Create captures only name + trigger, then redirects into the visual builder
// (edit) where the flow itself is designed — so the builder always has an id.
export function AutomationCreatePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { slug } = automationsCreateRoute.useParams()

  const form = useForm<AutomationDetailsValues>({
    initialValues: { name: '', triggerEvent: 'contact.created' },
  })

  const createMutation = useResourceMutation({
    mutation: siteAutomationsCreateMutation(),
    invalidate: [siteAutomationsListQueryKey({ path: { slug } })],
    successMessage: t(($) => $.notifications.automationCreated),
    errorTitle: t(($) => $.alerts.automationSaveErrorTitle),
    onDone: (created) =>
      navigate({ to: automationsEditRoute.to, params: { slug, automationId: created.id } }),
  })

  return (
    <Stack>
      <Title order={4}>{t(($) => $.automations.createTitle)}</Title>
      <form
        onSubmit={form.onSubmit((values) =>
          createMutation.mutate({
            path: { slug },
            body: { name: values.name.trim(), triggerEvent: values.triggerEvent },
          }),
        )}
      >
        <Stack>
          <AutomationDetailsFields form={form} />
          <Group justify="flex-end">
            <Button
              variant="default"
              type="button"
              onClick={() => void navigate({ to: automationsRoute.to, params: { slug } })}
            >
              {t(($) => $.actions.cancel)}
            </Button>
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.actions.continue)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Stack>
  )
}
