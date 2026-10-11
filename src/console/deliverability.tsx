import { Card, Group, Stack, Table, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { OperatorDeliverability, OperatorRate } from '../generated/operator/types.gen.ts'
import { resolveLocale } from '../i18n.ts'

// A rate as its percentage over its "<numerator> / <denominator>" counts: the counts always
// show, and the percentage is replaced by the floor notice while the rate is undefined
// (ADR 0011).
function RateCell({ rate, floor }: { rate: OperatorRate; floor: number }) {
  const { t } = useTranslation()

  return (
    <Stack gap={0}>
      {rate.rate === null ? (
        <Text size="sm" c="dimmed">
          {t(($) => $.console.workspace.rates.belowFloor, { floor })}
        </Text>
      ) : (
        <Text size="sm">
          {new Intl.NumberFormat(resolveLocale(), {
            style: 'percent',
            maximumFractionDigits: 2,
          }).format(rate.rate)}
        </Text>
      )}
      <Text size="xs" c="dimmed">
        {`${rate.numerator} / ${rate.denominator}`}
      </Text>
    </Stack>
  )
}

// The Workspace's Complaint and Bounce rates per Sending domain and its send volume over
// the trailing window: counts only, never content (ADR 0026).
export function DeliverabilityCard({ deliverability }: { deliverability: OperatorDeliverability }) {
  const { t } = useTranslation()

  return (
    <Card withBorder>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.console.workspace.rates.title)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.console.workspace.rates.window, { hours: deliverability.windowHours })}
        </Text>
      </Group>
      <Group gap="xs" mb="sm" align="baseline">
        <Text size="sm" c="dimmed">
          {t(($) => $.console.workspace.rates.sendVolume)}
        </Text>
        <Text size="lg" fw={600}>
          {new Intl.NumberFormat(resolveLocale()).format(deliverability.sendVolume)}
        </Text>
      </Group>
      {deliverability.domains.length === 0 ? (
        <Text size="sm" c="dimmed">
          {t(($) => $.console.workspace.rates.noDomains)}
        </Text>
      ) : (
        <Table.ScrollContainer minWidth={420}>
          <Table>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t(($) => $.console.workspace.rates.domain)}</Table.Th>
                <Table.Th>{t(($) => $.console.workspace.rates.complaintRate)}</Table.Th>
                <Table.Th>{t(($) => $.console.workspace.rates.bounceRate)}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {deliverability.domains.map((domain) => (
                <Table.Tr key={domain.sendingDomainId}>
                  <Table.Td>{domain.domain}</Table.Td>
                  <Table.Td>
                    <RateCell rate={domain.complaintRate} floor={deliverability.volumeFloor} />
                  </Table.Td>
                  <Table.Td>
                    <RateCell rate={domain.bounceRate} floor={deliverability.volumeFloor} />
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </Card>
  )
}
