import { Button, Card, Group, Stack, Textarea, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useTranslation } from 'react-i18next'

import {
  siteWorkspacesListQueryKey,
  siteWorkspacesUpdateMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$slug/settings/index.tsx'
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
    <Card withBorder>
      <Title order={4} mb="sm">
        {t(($) => $.settings.generalTitle)}
      </Title>
      <form
        onSubmit={form.onSubmit((values) =>
          updateMutation.mutate({
            path: { slug: workspace.slug },
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
    </Card>
  )
}

export function GeneralPage() {
  const { t } = useTranslation()
  const { slug } = generalRoute.useParams()

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.general)}>
      {(workspace) => <GeneralSection workspace={workspace} />}
    </SettingsFrame>
  )
}
