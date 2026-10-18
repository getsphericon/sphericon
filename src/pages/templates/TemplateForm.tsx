import { Button, Group, Stack, Textarea, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../../components/RouterLink.tsx'
import type { SiteCreateEmailTemplateInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$slug/route.tsx'
import { Route as templatesRoute } from '../../routes/workspaces/$slug/templates/index.tsx'

export function TemplateForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<SiteCreateEmailTemplateInput>) {
  const { t } = useTranslation()
  const { slug } = workspaceRoute.useParams()

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.templates.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <TextInput label={t(($) => $.templates.subjectLabel)} {...form.getInputProps('subject')} />
        <Textarea
          label={t(($) => $.templates.bodyLabel)}
          description={t(($) => $.templates.bodyHint)}
          autosize
          minRows={12}
          {...form.getInputProps('body')}
        />

        <Group justify="flex-end">
          <ButtonLink variant="default" to={templatesRoute.to} params={{ slug }}>
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
