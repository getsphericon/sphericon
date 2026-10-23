import { Box, Button, Group, Select, Stack, Textarea, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesListQueryKey,
  siteWorkspacesSetTimezoneMutation,
  siteWorkspacesUpdateMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$workspaceId/settings/index.tsx'
import { DangerZoneSection } from './DangerZoneSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// GeneralSection is split out so the rename form seeds its initial value from the
// loaded workspace without an effect.
function GeneralSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const form = useForm<{ name: string; postalAddress: string }>({
    initialValues: { name: workspace.name, postalAddress: workspace.postalAddress },
  })

  // Renaming is for Owner and Admin (ADR 0043); a Member sees the values read-only.
  const canEdit = workspace.role !== 'member'

  const updateMutation = useResourceMutation({
    mutation: siteWorkspacesUpdateMutation(),
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.settings.saved),
    errorTitle: t(($) => $.settings.errorTitle),
  })

  return (
    <Box>
      <form
        onSubmit={form.onSubmit((values) =>
          updateMutation.mutate({
            path: { workspaceId: workspace.id },
            body: { name: values.name.trim(), postalAddress: values.postalAddress.trim() },
          }),
        )}
      >
        <Stack maw={420}>
          <TextInput
            label={t(($) => $.settings.nameLabel)}
            required
            disabled={!canEdit}
            {...form.getInputProps('name')}
          />
          <Textarea
            label={t(($) => $.settings.postalAddressLabel)}
            description={t(($) => $.settings.postalAddressDescription)}
            placeholder={t(($) => $.settings.postalAddressPlaceholder)}
            autosize
            minRows={2}
            disabled={!canEdit}
            {...form.getInputProps('postalAddress')}
          />
          {canEdit ? (
            <Group justify="flex-end">
              <Button type="submit" loading={updateMutation.isPending}>
                {t(($) => $.actions.save)}
              </Button>
            </Group>
          ) : null}
        </Stack>
      </form>
    </Box>
  )
}

// The zones a browser knows; UTC is the default and not always among them.
const TIMEZONES = ['UTC', ...Intl.supportedValuesOf('timeZone').filter((zone) => zone !== 'UTC')]

// TimezoneSection sets the Workspace time zone (ADR 0057): the zone every report counts its days
// in. Only an Owner changes it; everyone else sees it read-only.
function TimezoneSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const form = useForm<{ timezone: string }>({ initialValues: { timezone: workspace.timezone } })
  const canEdit = workspace.role === 'owner'

  const mutation = useResourceMutation({
    mutation: siteWorkspacesSetTimezoneMutation(),
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.settings.saved),
    errorTitle: t(($) => $.settings.errorTitle),
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        mutation.mutate({
          path: { workspaceId: workspace.id },
          body: { timezone: values.timezone },
        }),
      )}
    >
      <Stack maw={420}>
        <Select
          label={t(($) => $.settings.timezoneLabel)}
          description={t(($) => $.settings.timezoneDescription)}
          data={TIMEZONES}
          searchable
          allowDeselect={false}
          disabled={!canEdit}
          {...form.getInputProps('timezone')}
        />
        {canEdit ? (
          <Group justify="flex-end">
            <Button type="submit" loading={mutation.isPending}>
              {t(($) => $.actions.save)}
            </Button>
          </Group>
        ) : null}
      </Stack>
    </form>
  )
}

export function GeneralPage() {
  const { t } = useTranslation()
  const { workspaceId } = generalRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.general)}>
      {(workspace) => (
        <Stack gap="xl">
          <GeneralSection workspace={workspace} />
          <TimezoneSection workspace={workspace} />
          <DangerZoneSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
