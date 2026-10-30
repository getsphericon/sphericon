import { Card, EmptyState, Stack, Table, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { StaffHold } from '../generated/staffee/types.gen.ts'
import { formatDateTime } from '../utils/datetime.ts'

// A Workspace's active Staff holds, read only: what the Staff member needs to answer a
// support question. The list arrives with the Workspace and holds no lapsed hold.
export function HoldsCard({ holds }: { holds: StaffHold[] }) {
  const { t } = useTranslation()

  return (
    <Card withBorder>
      <Stack>
        <Title order={4}>{t(($) => $.console.workspace.holds.title)}</Title>
        {holds.length === 0 ? (
          <EmptyState title={t(($) => $.console.workspace.holds.empty)} />
        ) : (
          <Table.ScrollContainer minWidth={640}>
            <Table withTableBorder>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>{t(($) => $.console.workspace.holds.key)}</Table.Th>
                  <Table.Th>{t(($) => $.console.workspace.holds.reasonCode)}</Table.Th>
                  <Table.Th>{t(($) => $.console.workspace.holds.message)}</Table.Th>
                  <Table.Th>{t(($) => $.console.workspace.holds.since)}</Table.Th>
                  <Table.Th>{t(($) => $.console.workspace.holds.expires)}</Table.Th>
                  <Table.Th>{t(($) => $.console.workspace.holds.setBy)}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {holds.map((hold) => (
                  <Table.Tr key={hold.key}>
                    <Table.Td>{hold.key}</Table.Td>
                    <Table.Td>{hold.reasonCode}</Table.Td>
                    <Table.Td>{hold.message}</Table.Td>
                    <Table.Td>{formatDateTime(hold.createdAt)}</Table.Td>
                    <Table.Td>{formatDateTime(hold.expiresAt)}</Table.Td>
                    <Table.Td>{hold.setBy}</Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </Stack>
    </Card>
  )
}
