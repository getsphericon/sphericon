import { Button, Group, Stack, TextInput } from '@mantine/core'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import type { CreateTemplateInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as templatesRoute } from '../../routes/workspaces/$workspaceId/templates/index.tsx'
import { docCodec } from '../../utils/messageDoc.ts'

export function TemplateForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<CreateTemplateInput>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  // The form holds the document as JSON text; a blank or unreadable text is no document.
  const docText = form.values.doc
  const doc = useMemo(() => {
    const parsed = docCodec.safeParse(docText)
    return parsed.success ? parsed.data : null
  }, [docText])
  // A message written in the editor is its document and the server builds the body; sending
  // both is a 422.
  const submit = (payload: CreateTemplateInput) => {
    const { doc: written, name, subject } = payload
    onSubmit(written ? { name, ...(subject !== undefined && { subject }), doc: written } : payload)
  }

  return (
    <form onSubmit={form.onSubmit(submit)}>
      <Stack>
        <TextInput
          label={t(($) => $.templates.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <TextInput label={t(($) => $.templates.subjectLabel)} {...form.getInputProps('subject')} />
        <MessageEditor
          doc={doc}
          body={form.values.body ?? ''}
          onChange={(edit) => {
            if ('doc' in edit) {
              form.setFieldValue('doc', z.encode(docCodec, edit.doc))
            } else {
              form.setFieldValue('doc', '')
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
