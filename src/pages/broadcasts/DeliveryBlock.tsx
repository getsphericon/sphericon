import { Button, Divider, Group, Text, TextInput } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteBroadcastsGetQueryKey,
  siteBroadcastsListQueryKey,
  siteBroadcastsScheduleMutation,
  siteBroadcastsSendMutation,
  siteBroadcastsTestSendMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// Send now, schedule and test send for a loaded Broadcast. Sending and scheduling are only
// possible for drafts; test send works for any status of an email Broadcast, to an address, and of
// an SMS one, to a phone number: Telegram has none, so its action is not offered (ADR 0080).
export function DeliveryBlock({ broadcast }: { broadcast: SiteBroadcastResource }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId } = workspaceRoute.useParams()
  const [scheduledAt, setScheduledAt] = useState('')
  // Where the Test send goes: an address on email, a phone number on SMS.
  const [testTo, setTestTo] = useState('')

  const path = { workspaceId, id: broadcast.id }
  const isDraft = broadcast.status === 'draft'
  const invalidate = [
    siteBroadcastsListQueryKey({ path: { workspaceId } }),
    siteBroadcastsGetQueryKey({ path }),
  ]
  const toReport = async () =>
    navigate({ to: broadcastsReportRoute.to, params: { workspaceId, broadcastId: broadcast.id } })

  const sendMutation = useResourceMutation({
    mutation: siteBroadcastsSendMutation(),
    invalidate,
    successMessage: t(($) => $.notifications.broadcastSent),
    errorTitle: t(($) => $.alerts.broadcastSendErrorTitle),
    onDone: toReport,
  })

  const scheduleMutation = useResourceMutation({
    mutation: siteBroadcastsScheduleMutation(),
    invalidate,
    successMessage: t(($) => $.notifications.broadcastScheduled),
    errorTitle: t(($) => $.alerts.broadcastSendErrorTitle),
    onDone: toReport,
  })

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
      <Group align="flex-end">
        <Button
          color="teal"
          disabled={!isDraft}
          loading={sendMutation.isPending}
          onClick={() => sendMutation.mutate({ path })}
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
          disabled={!isDraft || scheduledAt === ''}
          loading={scheduleMutation.isPending}
          onClick={() =>
            scheduleMutation.mutate({
              path,
              body: { scheduledAt: new Date(scheduledAt).toISOString() },
            })
          }
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
