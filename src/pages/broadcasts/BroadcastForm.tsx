import { Alert, Button, Group, Select, Stack, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteSegmentsListOptions,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteCreateBroadcastInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { docCodec } from '../../utils/messageDoc.ts'

export function BroadcastForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<SiteCreateBroadcastInput>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()

  const segmentsQuery = useQuery(
    siteSegmentsListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
  )
  const segmentOptions = [
    { value: '', label: t(($) => $.broadcasts.audienceAll) },
    ...(segmentsQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name })),
  ]

  const templatesQuery = useQuery(
    siteTemplatesListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
  )
  const templates = templatesQuery.data?.items ?? []

  // The form holds the document as JSON text; a blank or unreadable text is no document.
  const docText = form.values.doc
  const doc = useMemo(() => {
    const parsed = docCodec.safeParse(docText)
    return parsed.success ? parsed.data : null
  }, [docText])
  // The editor reads its content once when it mounts, so a copied Template remounts it.
  const [editorKey, setEditorKey] = useState(0)

  // A copy, never a reference (ADR 0003): the Template's subject, document and body are written
  // into the form, and editing the Template afterwards changes nothing here.
  const applyTemplate = (id: string | null) => {
    const tpl = templates.find((x) => x.id === id)
    if (!tpl) {
      return
    }
    if (tpl.subject) {
      form.setFieldValue('subject', tpl.subject)
    }
    form.setFieldValue('body', tpl.body)
    form.setFieldValue('doc', tpl.doc ? z.encode(docCodec, tpl.doc) : '')
    setEditorKey((key) => key + 1)
  }

  // A message written in the editor is its document and the server builds the body; sending
  // both is a 422.
  const submit = (payload: SiteCreateBroadcastInput) => {
    const { doc: written, name, subject, fromName, fromEmail, segmentId } = payload
    onSubmit(
      written
        ? {
            name,
            doc: written,
            ...(subject !== undefined && { subject }),
            ...(fromName !== undefined && { fromName }),
            ...(fromEmail !== undefined && { fromEmail }),
            ...(segmentId !== undefined && { segmentId }),
          }
        : payload,
    )
  }

  return (
    <form onSubmit={form.onSubmit(submit)}>
      <Stack>
        <TextInput
          label={t(($) => $.broadcasts.nameLabel)}
          required
          {...form.getInputProps('name')}
        />
        <TextInput label={t(($) => $.broadcasts.subjectLabel)} {...form.getInputProps('subject')} />
        <Group grow align="flex-start">
          <TextInput
            label={t(($) => $.broadcasts.fromNameLabel)}
            {...form.getInputProps('fromName')}
          />
          <TextInput
            label={t(($) => $.broadcasts.fromEmailLabel)}
            type="email"
            {...form.getInputProps('fromEmail')}
          />
        </Group>

        <Select
          label={t(($) => $.broadcasts.audienceLabel)}
          data={segmentOptions}
          allowDeselect={false}
          {...form.getInputProps('segmentId')}
        />
        <Alert color="blue" variant="light">
          {t(($) => $.broadcasts.audienceNote)}
        </Alert>

        {templates.length > 0 ? (
          <Select
            label={t(($) => $.broadcasts.useTemplate)}
            placeholder={t(($) => $.broadcasts.useTemplateNone)}
            clearable
            data={templates.map((tpl) => ({ value: tpl.id, label: tpl.name }))}
            onChange={applyTemplate}
          />
        ) : null}

        <MessageEditor
          key={editorKey}
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
          <ButtonLink variant="default" to={broadcastsRoute.to} params={{ workspaceId }}>
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
