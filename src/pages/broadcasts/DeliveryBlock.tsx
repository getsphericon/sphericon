import { Button, Divider, Group, Text, TextInput } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { siteBroadcastsTestSendMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteBroadcastResource,
  ScheduleBroadcastInput,
} from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import type { ResourceEditor } from '../../resources/defineResource.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// The operations of a Broadcast's editor session that deliver it.
export type BroadcastDelivery = 'SEND' | 'SCHEDULE'

// Send now, schedule and test send for a loaded Broadcast. Sending and scheduling run through the
// editor's session: they deliver the saved content, so they wait for a pending Save and for the
// form's unsaved edits to be saved, and are only possible for drafts. Test send works for any
// status of an email Broadcast, to an address, and of an SMS one, to a phone number: Telegram has
// none, so its action is not offered (ADR 0080).
export function DeliveryBlock({
  broadcast,
  editor,
}: {
  broadcast: SiteBroadcastResource
  editor: ResourceEditor<BroadcastDelivery>
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId } = workspaceRoute.useParams()
  const [scheduledAt, setScheduledAt] = useState('')
  // Where the Test send goes: an address on email, a phone number on SMS.
  const [testTo, setTestTo] = useState('')

  const path = { workspaceId, id: broadcast.id }
  // A delivered Broadcast is followed on its report.
  const deliver = async (operation: BroadcastDelivery, input?: ScheduleBroadcastInput) => {
    if ((await editor.run(operation, input)) === 'success') {
      await navigate({
        to: broadcastsReportRoute.to,
        params: { workspaceId, broadcastId: broadcast.id },
      })
    }
  }

  const testSendMutation = useResourceMutation({
    mutation: siteBroadcastsTestSendMutation(),
    successMessage:
      broadcast.channel === 'sms'
        ? t(($) => $.notifications.testTextSent)
        : t(($) => $.notifications.testSent),
    errorTitle: t(($) => $.alerts.broadcastSendErrorTitle),
  })

  return (
    <>
      <Divider label={t(($) => $.broadcasts.deliveryLabel)} />
      <Text size="sm" c="dimmed">
        {t(($) => $.broadcasts.deliveryHint)}
      </Text>
      {editor.dirty && broadcast.status === 'draft' && (
        <Text size="sm" c="dimmed">
          {t(($) => $.broadcasts.deliveryUnsaved)}
        </Text>
      )}
      <Group align="flex-end">
        <Button
          color="teal"
          disabled={!editor.can('SEND')}
          loading={editor.operation === 'SEND'}
          onClick={() => {
            void deliver('SEND')
          }}
        >
          {t(($) => $.broadcasts.sendNow)}
        </Button>
        <TextInput
          type="datetime-local"
          label={t(($) => $.broadcasts.scheduleLabel)}
          value={scheduledAt}
          onChange={(e) => setScheduledAt(e.currentTarget.value)}
        />
        <Button
          variant="light"
          disabled={!editor.can('SCHEDULE') || scheduledAt === ''}
          loading={editor.operation === 'SCHEDULE'}
          onClick={() => {
            void deliver('SCHEDULE', { scheduledAt: new Date(scheduledAt).toISOString() })
          }}
        >
          {t(($) => $.broadcasts.schedule)}
        </Button>
      </Group>

      {broadcast.channel === 'email' && (
        <>
          <Group align="flex-end">
            <TextInput
              type="email"
              label={t(($) => $.broadcasts.testSendEmailLabel)}
              value={testTo}
              onChange={(e) => setTestTo(e.currentTarget.value)}
            />
            <Button
              variant="default"
              disabled={testTo === ''}
              loading={testSendMutation.isPending}
              onClick={() => testSendMutation.mutate({ path, body: { email: testTo } })}
            >
              {t(($) => $.broadcasts.testSendButton)}
            </Button>
          </Group>
          {testSendMutation.data && (
            <Text size="sm" c="dimmed">
              {testSendMutation.data.captured
                ? t(($) => $.broadcasts.testSendCaptured)
                : t(($) => $.broadcasts.testSendFrom, { from: testSendMutation.data.from ?? '' })}
            </Text>
          )}
        </>
      )}
      {broadcast.channel === 'sms' && (
        <Group align="flex-end">
          <TextInput
            type="tel"
            label={t(($) => $.broadcasts.testSendPhoneLabel)}
            description={t(($) => $.broadcasts.testSendPhoneHint)}
            value={testTo}
            onChange={(e) => setTestTo(e.currentTarget.value)}
          />
          <Button
            variant="default"
            disabled={testTo === ''}
            loading={testSendMutation.isPending}
            onClick={() => testSendMutation.mutate({ path, body: { phone: testTo } })}
          >
            {t(($) => $.broadcasts.testSendButton)}
          </Button>
        </Group>
      )}
    </>
  )
}
