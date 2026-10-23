import { Button, Group, Modal, Stack, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteAutomationsCreateMutation,
  siteAutomationsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { DEFAULT_TRIGGER_EVENT } from './nodes.ts'

// Create captures only the name, then redirects into the visual builder (edit) where the
// trigger and the steps are designed on the canvas — so the builder always has an id.
export function AutomationCreateModal({
  opened,
  onClose,
}: {
  opened: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId } = automationsRoute.useParams()

  const form = useForm({
    initialValues: { name: '' },
  })

  const createMutation = useResourceMutation({
    mutation: siteAutomationsCreateMutation(),
    invalidate: [siteAutomationsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.notifications.automationCreated),
    errorTitle: t(($) => $.alerts.automationSaveErrorTitle),
    onDone: async (created) =>
      navigate({ to: automationsEditRoute.to, params: { workspaceId, automationId: created.id } }),
  })

  return (
    <Modal opened={opened} onClose={onClose} title={t(($) => $.automations.createTitle)}>
      <form
        onSubmit={form.onSubmit((values) =>
          createMutation.mutate({
            path: { workspaceId },
            body: { name: values.name.trim(), triggerEvent: DEFAULT_TRIGGER_EVENT },
          }),
        )}
      >
        <Stack>
          <TextInput
            label={t(($) => $.automations.nameLabel)}
            required
            data-autofocus
            {...form.getInputProps('name')}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              {t(($) => $.actions.cancel)}
            </Button>
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.actions.continue)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  )
}
