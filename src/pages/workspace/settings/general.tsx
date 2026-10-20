import { Box, Button, Group, Stack, Textarea, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesListQueryKey,
  siteWorkspacesUpdateMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$workspaceId/settings/index.tsx'
import { BrandKitSection } from '../BrandKitSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// GeneralSection is split out so the rename form seeds its initial value from the
// loaded workspace without an effect.
function GeneralSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const form = useForm<{ name: string; postalAddress: string }>({
    initialValues: { name: workspace.name, postalAddress: workspace.postalAddress },
  })

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
            {...form.getInputProps('name')}
          />
          <Textarea
            label={t(($) => $.settings.postalAddressLabel)}
            description={t(($) => $.settings.postalAddressDescription)}
            placeholder={t(($) => $.settings.postalAddressPlaceholder)}
            autosize
            minRows={2}
            {...form.getInputProps('postalAddress')}
          />
          <Group justify="flex-end">
            <Button type="submit" loading={updateMutation.isPending}>
              {t(($) => $.actions.save)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Box>
  )
}

export function GeneralPage() {
  const { t } = useTranslation()
  const { workspaceId } = generalRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.general)}>
      {(workspace) => (
        <Stack>
          <GeneralSection workspace={workspace} />
          <BrandKitSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
