import { Button, Group, Stack, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteCreateContactInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$slug/contacts/index.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$slug/route.tsx'

// Identity is multi-key: subject_id / email / phone are alias keys, any of which may be
// absent (ADR 0002); blank inputs are normalized away by the contact form schema.
export function ContactForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<SiteCreateContactInput>) {
  const { t } = useTranslation()
  const { slug } = workspaceRoute.useParams()

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput label={t(($) => $.table.email)} {...form.getInputProps('email')} />
        <TextInput label={t(($) => $.table.subjectId)} {...form.getInputProps('subjectId')} />
        <TextInput label={t(($) => $.table.phone)} {...form.getInputProps('phone')} />
        <TextInput label={t(($) => $.table.firstName)} {...form.getInputProps('firstName')} />
        <TextInput label={t(($) => $.table.lastName)} {...form.getInputProps('lastName')} />
        <TextInput label={t(($) => $.table.timeZone)} {...form.getInputProps('timeZone')} />

        <Group justify="flex-end">
          <ButtonLink variant="default" to={contactsRoute.to} params={{ slug }}>
            {t(($) => $.actions.cancel)}
          </ButtonLink>
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
