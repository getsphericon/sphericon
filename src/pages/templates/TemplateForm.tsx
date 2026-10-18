import { Button, Group, Stack, Textarea, TextInput } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

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
  const navigate = useNavigate()
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
          <Button
            variant="default"
            type="button"
            onClick={() => void navigate({ to: templatesRoute.to, params: { slug } })}
          >
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
