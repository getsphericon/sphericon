import { Alert, Button, Group, Select, Stack, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { AnchorLink, ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteIntegrationsListOptions,
  siteSegmentsListOptions,
  siteSendingDomainsListOptions,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { CreateBroadcastInput } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as emailsRoute } from '../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { joinEmail, splitEmail } from '../../utils/emailAddress.ts'
import { docCodec } from '../../utils/messageDoc.ts'
import { SendCapWarning } from './SendCapWarning.tsx'

export function BroadcastForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<CreateBroadcastInput>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()

  const segmentsQuery = useQuery(
    siteSegmentsListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
  )
  const segmentOptions = [
    { value: '', label: t(($) => $.broadcasts.segmentAll) },
    ...(segmentsQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name })),
  ]

  // The Integration the broadcast sends and paces through; blank is the Workspace default.
  const integrationsQuery = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
  )
  const integrationOptions = [
    { value: '', label: t(($) => $.broadcasts.integrationDefault) },
    ...(integrationsQuery.data ?? [])
      .filter((integration) => integration.channel === 'email')
      .map((integration) => ({ value: integration.id, label: integration.name })),
  ]

  // From is a local part on one of the verified Sending domains (ADR 0010); the server is the
  // source of truth and refuses another. A saved From whose domain lost verification stays
  // visible, marked, so the reason for a hold can be seen.
  const domainsQuery = useQuery(
    siteSendingDomainsListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
  )
  const verifiedDomains = (domainsQuery.data?.items ?? [])
    .filter((d) => d.verified)
    .map((d) => d.domain)
  const { local: fromLocal, domain: savedDomain } = splitEmail(form.values.fromEmail ?? '')
  const [chosenDomain, setChosenDomain] = useState('')
  const fromDomain = savedDomain || chosenDomain || (verifiedDomains[0] ?? '')
  const domainOptions = [
    ...verifiedDomains.map((domain) => ({ value: domain, label: domain })),
    ...(savedDomain && !verifiedDomains.includes(savedDomain)
      ? [
          {
            value: savedDomain,
            label: t(($) => $.broadcasts.fromNotVerified, { domain: savedDomain }),
          },
        ]
      : []),
  ]
  // An empty local part is an empty From: the Integration's default.
  const setFrom = (local: string, domain: string) => {
    setChosenDomain(domain)
    form.setFieldValue('fromEmail', joinEmail(local, domain))
  }

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
  const submit = (payload: CreateBroadcastInput) => {
    const { doc: written, name, subject, fromName, fromEmail, segmentId, integrationId } = payload
    onSubmit(
      written
        ? {
            name,
            doc: written,
            ...(subject !== undefined && { subject }),
            ...(fromName !== undefined && { fromName }),
            ...(fromEmail !== undefined && { fromEmail }),
            ...(segmentId !== undefined && { segmentId }),
            ...(integrationId !== undefined && { integrationId }),
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
            value={fromLocal}
            disabled={domainOptions.length === 0}
            onChange={(e) => setFrom(e.currentTarget.value.trim(), fromDomain)}
            error={form.errors.fromEmail}
          />
          <Select
            label={t(($) => $.broadcasts.fromDomainLabel)}
            data={domainOptions}
            value={fromDomain}
            disabled={domainOptions.length === 0}
            allowDeselect={false}
            onChange={(domain) => setFrom(fromLocal, domain ?? '')}
          />
        </Group>
        {domainsQuery.isSuccess && verifiedDomains.length === 0 ? (
          <Alert color="blue" variant="light">
            {t(($) => $.broadcasts.fromNoDomain)}{' '}
            <AnchorLink to={emailsRoute.to} params={{ workspaceId }}>
              {t(($) => $.broadcasts.fromNoDomainLink)}
            </AnchorLink>
          </Alert>
        ) : null}

        <Select
          label={t(($) => $.broadcasts.integrationLabel)}
          data={integrationOptions}
          allowDeselect={false}
          {...form.getInputProps('integrationId')}
        />

        <Select
          label={t(($) => $.broadcasts.segmentLabel)}
          data={segmentOptions}
          allowDeselect={false}
          {...form.getInputProps('segmentId')}
        />
        <Alert color="blue" variant="light">
          {t(($) => $.broadcasts.segmentNote)}
        </Alert>
        <SendCapWarning
          workspaceId={workspaceId}
          segmentId={form.values.segmentId}
          segments={segmentsQuery.data?.items}
        />

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
