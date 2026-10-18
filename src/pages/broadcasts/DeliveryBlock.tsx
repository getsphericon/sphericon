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
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/report.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$slug/route.tsx'

// Send now, schedule and test send for a loaded Broadcast. Sending and scheduling are only
// possible for drafts; test send works for any status.
export function DeliveryBlock({ broadcast }: { broadcast: SiteBroadcastResource }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { slug } = workspaceRoute.useParams()
  const [scheduledAt, setScheduledAt] = useState('')
  const [testEmail, setTestEmail] = useState('')

  const path = { slug, id: broadcast.id }
  const isDraft = broadcast.status === 'draft'
  const invalidate = [
    siteBroadcastsListQueryKey({ path: { slug } }),
    siteBroadcastsGetQueryKey({ path }),
  ]
  const toReport = () =>
    navigate({ to: broadcastsReportRoute.to, params: { slug, broadcastId: broadcast.id } })

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
    successMessage: t(($) => $.notifications.testSent),
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

      <Group align="flex-end">
        <TextInput
          type="email"
          label={t(($) => $.broadcasts.testSendEmailLabel)}
          value={testEmail}
          onChange={(e) => setTestEmail(e.currentTarget.value)}
        />
        <Button
          variant="default"
          disabled={testEmail === ''}
          loading={testSendMutation.isPending}
          onClick={() => testSendMutation.mutate({ path, body: { email: testEmail } })}
        >
          {t(($) => $.broadcasts.testSendButton)}
        </Button>
      </Group>
    </>
  )
}
