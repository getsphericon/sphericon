import { Button, Group, Stack, Table, Text, Title } from '@mantine/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { DetailsBadge } from '../../components/DetailsBadge.tsx'
import {
  siteEngagementTrackingCreateMutation,
  siteEngagementTrackingGetOptions,
  siteEngagementTrackingGetQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  EngagementTrackingChannel,
  EngagementTrackingConsent,
  EngagementTrackingProvenance,
  EngagementTrackingState,
} from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useFormat } from '../../hooks/useFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

interface Props {
  workspaceId: string
  contactId: string
}

// Whether the opens and clicks of the contact's messages are tracked, per Destination (ADR 0120):
// the consent recorded with who recorded it, or the Workspace's default and what it means now.
// A member records a refusal the person made by another route (a call, a reply) or a grant; a
// refusal can be lifted only by the person (ADR 0121), so Grant is disabled over one, with the
// reason beside it. The Destinations come from the read, never from the contact's fields: the
// consent is keyed by the Destination.
export function TrackingSection({ workspaceId, contactId }: Props) {
  const { t } = useTranslation()
  const path = { workspaceId, contactId }
  const consents = useQuery(siteEngagementTrackingGetOptions({ path }))
  const destinations = consents.data?.destinations ?? []

  if (destinations.length === 0) {
    return null
  }

  return (
    <Stack>
      <Title order={5}>{t(($) => $.contacts.trackingTitle)}</Title>
      <Text c="dimmed" size="sm">
        {t(($) => $.contacts.trackingHint)}
      </Text>
      <Table.ScrollContainer minWidth={480}>
        <Table withTableBorder>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.contacts.trackingDestination)}</Table.Th>
              <Table.Th>{t(($) => $.contacts.trackingState)}</Table.Th>
              <Table.Th />
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {destinations.map((consent) => (
              <TrackingRow
                key={`${consent.channel}:${consent.destination}`}
                path={path}
                consent={consent}
              />
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}

// The color of each reading's badge, beside its word: a refusal red, a grant teal, the default gray.
const stateColors: Record<EngagementTrackingConsent['state'], string> = {
  granted: 'teal',
  denied: 'red',
  default: 'gray',
}

function TrackingRow({
  path,
  consent,
}: {
  path: { workspaceId: string; contactId: string }
  consent: EngagementTrackingConsent
}) {
  const { t } = useTranslation()
  const { formatDateTime } = useFormat()
  const confirmRefusal = useConfirmation()
  const queryClient = useQueryClient()
  // A refused observation (a grant over a refusal recorded meanwhile) reads the consent again, so
  // the row shows what stands; the toast still says why.
  const reread = async () => {
    await queryClient.invalidateQueries({ queryKey: siteEngagementTrackingGetQueryKey({ path }) })
    return false
  }

  const channels: Record<EngagementTrackingChannel, string> = {
    email: t(($) => $.contacts.trackingChannelEmail),
    sms: t(($) => $.contacts.trackingChannelSms),
    telegram: t(($) => $.contacts.trackingChannelTelegram),
  }
  const provenances: Record<EngagementTrackingProvenance, string> = {
    page: t(($) => $.contacts.trackingProvenancePage),
    manual: t(($) => $.contacts.trackingProvenanceManual),
    imported: t(($) => $.contacts.trackingProvenanceImported),
    onsite_form: t(($) => $.contacts.trackingProvenanceOnsiteForm),
  }

  const grant = useResourceMutation({
    mutation: siteEngagementTrackingCreateMutation(),
    successMessage: t(($) => $.contacts.trackingGrantedRecorded),
    errorTitle: t(($) => $.contacts.trackingError),
    handleError: reread,
  })
  const refuse = useResourceMutation({
    mutation: siteEngagementTrackingCreateMutation(),
    successMessage: t(($) => $.contacts.trackingRefused),
    errorTitle: t(($) => $.contacts.trackingError),
    handleError: reread,
  })
  const body = (state: EngagementTrackingState) => ({
    channel: consent.channel,
    destination: consent.destination,
    state,
  })
  const onRefuse = () =>
    confirmRefusal({
      title: t(($) => $.contacts.trackingRefuseConfirmTitle),
      description: t(($) => $.contacts.trackingRefuseConfirmDescription, {
        destination: consent.destination,
      }),
      confirmLabel: t(($) => $.contacts.trackingRefuse),
      onConfirm: () => refuse.mutate({ path, body: body('denied') }),
    })

  const status = {
    granted: t(($) => $.contacts.trackingGranted),
    denied: t(($) => $.contacts.trackingDenied),
    default:
      consent.effective === 'track'
        ? t(($) => $.contacts.trackingDefaultTrack)
        : t(($) => $.contacts.trackingDefaultDontTrack),
  }[consent.state]
  const details = consent.provenance
    ? [
        provenances[consent.provenance],
        consent.updatedAt
          ? t(($) => $.contacts.trackingSince, { at: formatDateTime(consent.updatedAt) })
          : null,
      ]
    : [t(($) => $.contacts.trackingDefaultDetails)]
  const denied = consent.state === 'denied'

  return (
    <Table.Tr>
      <Table.Td>
        <Text size="sm">{consent.destination}</Text>
        <Text c="dimmed" size="xs">
          {channels[consent.channel]}
        </Text>
      </Table.Td>
      <Table.Td>
        <DetailsBadge variant="light" color={stateColors[consent.state]} details={details}>
          {status}
        </DetailsBadge>
      </Table.Td>
      <Table.Td>
        <Group gap="xs" justify="flex-end" wrap="nowrap">
          {denied ? (
            <Text c="dimmed" size="sm">
              {t(($) => $.contacts.trackingOnlyThePerson)}
            </Text>
          ) : null}
          {denied ? null : (
            <Button
              variant="default"
              color="red"
              size="xs"
              loading={refuse.isPending}
              onClick={onRefuse}
            >
              {t(($) => $.contacts.trackingRefuse)}
            </Button>
          )}
          {consent.state === 'granted' ? null : (
            <Button
              variant="default"
              size="xs"
              disabled={denied}
              loading={grant.isPending}
              onClick={() => grant.mutate({ path, body: body('granted') })}
            >
              {t(($) => $.contacts.trackingGrant)}
            </Button>
          )}
        </Group>
      </Table.Td>
    </Table.Tr>
  )
}
