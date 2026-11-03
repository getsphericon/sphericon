import { Alert, Button, Group, Radio, Select, Stack, Text, TextInput } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MessageEditor } from '../../components/MessageEditor.tsx'
import { AnchorLink, ButtonLink } from '../../components/RouterLink.tsx'
import { TelegramContentEditor } from '../../components/telegram/TelegramContentEditor.tsx'
import { TelegramUnsubscribeControl } from '../../components/telegram/TelegramUnsubscribeControl.tsx'
import {
  siteIntegrationsListOptions,
  siteSegmentsListOptions,
  siteSendingDomainsListOptions,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  BroadcastChannel,
  CreateBroadcastInput,
  TelegramButton,
} from '../../generated/site/types.gen.ts'
import { zBroadcastChannel } from '../../generated/site/zod.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as emailsRoute } from '../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { joinEmail, splitEmail } from '../../utils/emailAddress.ts'
import type { MessageDoc } from '../../utils/messageDoc.ts'
import { SendCapWarning } from './SendCapWarning.tsx'

export interface BroadcastValues {
  name: string
  // Blank until a new Broadcast's Channel is chosen; fixed from then on.
  channel: string
  subject: string
  fromName: string
  fromEmail: string
  segmentId: string
  integrationId: string
  body: string
  doc: MessageDoc | null
  text: string
  imageUrl: string
  buttons: TelegramButton[]
  linkPreview: boolean
  unsubscribeButton: string
}

// A new Broadcast asks for its Channel first (ADR 0080), then shows that Channel's editor; the
// Channel is fixed from then on, so an existing Broadcast opens on its editor.
export function BroadcastForm(props: ResourceFormProps<CreateBroadcastInput, BroadcastValues>) {
  const { form } = props
  const channel = zBroadcastChannel.safeParse(form.values.channel)
  if (!channel.success) {
    return <ChannelStep onChoose={(chosen) => form.setFieldValue('channel', chosen)} />
  }
  if (channel.data === 'telegram') {
    return <TelegramBroadcastForm {...props} />
  }
  return <EmailBroadcastForm {...props} />
}

// The Channels a Broadcast can be sent on: only those the Workspace has an enabled Integration
// for. With one, it is chosen already; the person still sees which.
function ChannelStep({ onChoose }: { onChoose: (channel: BroadcastChannel) => void }) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const integrationsQuery = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
  )
  const integrations = integrationsQuery.data ?? []
  const available = zBroadcastChannel.options.filter((channel) =>
    integrations.some((integration) => integration.enabled && integration.channel === channel),
  )
  const [chosen, setChosen] = useState<BroadcastChannel | null>(null)
  const only = available.length === 1 ? available[0] : undefined
  const value = chosen ?? only ?? null

  return (
    <Stack>
      <Radio.Group
        label={t(($) => $.broadcasts.channelLabel)}
        description={t(($) => $.broadcasts.channelHint)}
        value={value}
        onChange={(next) => setChosen(zBroadcastChannel.parse(next))}
      >
        <Stack gap="xs" mt="xs">
          {available.map((channel) => (
            <Radio key={channel} value={channel} label={t(($) => $.broadcasts.channels[channel])} />
          ))}
        </Stack>
      </Radio.Group>
      {integrationsQuery.isSuccess && available.length === 0 ? (
        <Alert color="blue" variant="light">
          {t(($) => $.broadcasts.channelNone)}{' '}
          <AnchorLink to={emailsRoute.to} params={{ workspaceId }}>
            {t(($) => $.broadcasts.channelNoneLink)}
          </AnchorLink>
        </Alert>
      ) : null}
      <Group justify="flex-end">
        <ButtonLink variant="default" to={broadcastsRoute.to} params={{ workspaceId }}>
          {t(($) => $.actions.cancel)}
        </ButtonLink>
        <Button
          disabled={value === null}
          onClick={() => {
            if (value !== null) {
              onChoose(value)
            }
          }}
        >
          {t(($) => $.actions.continue)}
        </Button>
      </Group>
    </Stack>
  )
}

// The editor of a Telegram Broadcast: its Integration, Segment and message. Who it reaches is the
// Segment's Contacts who have a Binding on the bot, so the note says that instead of email's.
function TelegramBroadcastForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<CreateBroadcastInput, BroadcastValues>) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()

  const segmentsQuery = useQuery(
    siteSegmentsListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
  )
  const segmentOptions = [
    { value: '', label: t(($) => $.broadcasts.segmentAll) },
    ...(segmentsQuery.data?.items ?? []).map((s) => ({ value: s.id, label: s.name })),
  ]

  // The bot the broadcast sends and paces through; blank is the Channel's default.
  const integrationsQuery = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
  )
  const integrationOptions = [
    { value: '', label: t(($) => $.broadcasts.integrationDefault) },
    ...(integrationsQuery.data ?? [])
      .filter((integration) => integration.channel === 'telegram')
      .map((integration) => ({ value: integration.id, label: integration.name })),
  ]

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <Text size="sm">
          {t(($) => $.broadcasts.channelShown, {
            channel: t(($) => $.broadcasts.channels.telegram),
          })}
        </Text>
        <TextInput
          label={t(($) => $.broadcasts.nameLabel)}
          required
          {...form.getInputProps('name')}
        />
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
          {t(($) => $.broadcasts.segmentNoteTelegram)}
        </Alert>
        <SendCapWarning
          workspaceId={workspaceId}
          segmentId={form.values.segmentId}
          segments={segmentsQuery.data?.items}
        />
        <TelegramContentEditor
          value={{
            text: form.values.text,
            imageUrl: form.values.imageUrl,
            buttons: form.values.buttons,
            linkPreview: form.values.linkPreview,
          }}
          onChange={(next) => {
            form.setFieldValue('text', next.text)
            form.setFieldValue('imageUrl', next.imageUrl)
            form.setFieldValue('buttons', next.buttons)
            form.setFieldValue('linkPreview', next.linkPreview)
          }}
        >
          <TelegramUnsubscribeControl
            label={form.values.unsubscribeButton}
            onChange={(label) => form.setFieldValue('unsubscribeButton', label)}
          />
        </TelegramContentEditor>
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

// The editor of an email Broadcast: its From, Integration, Segment and message.
function EmailBroadcastForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<CreateBroadcastInput, BroadcastValues>) {
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
  const { local: fromLocal, domain: savedDomain } = splitEmail(form.values.fromEmail)
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
    form.setFieldValue('doc', tpl.doc ?? null)
    setEditorKey((key) => key + 1)
  }

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <Text size="sm">
          {t(($) => $.broadcasts.channelShown, { channel: t(($) => $.broadcasts.channels.email) })}
        </Text>
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
