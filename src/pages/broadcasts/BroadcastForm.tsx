import { Alert, Button, Group, Select, Stack, Textarea, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteSegmentsListOptions,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteCreateBroadcastInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$slug/broadcasts/index.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$slug/route.tsx'

export function BroadcastForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<SiteCreateBroadcastInput>) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { slug } = workspaceRoute.useParams()

  const segmentsQuery = useQuery(
    siteSegmentsListOptions({ path: { slug }, query: { pageSize: 100 } }),
  )
  const segmentOptions = [
    { value: '', label: t(($) => $.broadcasts.audienceAll) },
    ...(segmentsQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name })),
  ]

  const templatesQuery = useQuery(
    siteTemplatesListOptions({ path: { slug }, query: { pageSize: 100 } }),
  )
  const templates = templatesQuery.data?.items ?? []

  const applyTemplate = (id: string | null) => {
    const tpl = templates.find((x) => x.id === id)
    if (!tpl) {
      return
    }
    if (tpl.subject) {
      form.setFieldValue('subject', tpl.subject)
    }
    form.setFieldValue('body', tpl.body)
  }

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
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

        <Textarea
          label={t(($) => $.broadcasts.bodyLabel)}
          description={t(($) => $.broadcasts.bodyHint)}
          autosize
          minRows={12}
          {...form.getInputProps('body')}
        />

        <Group justify="flex-end">
          <Button
            variant="default"
            type="button"
            onClick={() => void navigate({ to: broadcastsRoute.to, params: { slug } })}
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
