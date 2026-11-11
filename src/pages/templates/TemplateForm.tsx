import { Button, Group, Stack, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import type { CreateTemplateInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as templatesRoute } from '../../routes/workspaces/$workspaceId/templates/index.tsx'
import type { MessageDoc } from '../../utils/messageDoc.ts'

// What the form holds: the create input with every text given, and no document until one is made.
export type TemplateValues = Required<Pick<CreateTemplateInput, 'name' | 'subject' | 'body'>> & {
  doc: MessageDoc | null
}

export function TemplateForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<CreateTemplateInput, TemplateValues>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.templates.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <TextInput label={t(($) => $.templates.subjectLabel)} {...form.getInputProps('subject')} />
        <MessageEditor
          doc={form.values.doc}
          body={form.values.body}
          onChange={(edit) => {
            if ('doc' in edit) {
              form.setFieldValue('doc', edit.doc)
            } else {
              form.setFieldValue('doc', null)
              form.setFieldValue('body', edit.body)
            }
          }}
        />

        <Group justify="flex-end">
          <ButtonLink variant="default" to={templatesRoute.to} params={{ workspaceId }}>
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
