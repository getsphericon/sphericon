import { Stack, Table, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteLinkClicks } from '../generated/site/types.gen.ts'

/**
 * The clicked links of a Broadcast or an Automation, with how often each was. Renders nothing
 * while no click is recorded: the figures above stay the report.
 */
export function LinkClickTable({ links }: { links: SiteLinkClicks[] }) {
  const { t } = useTranslation()

  if (links.length === 0) {
    return null
  }

  return (
    <Stack gap="xs">
      <Title order={5}>{t(($) => $.linkClicks.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.linkClicks.hint)}
      </Text>
      <Table.ScrollContainer minWidth={320}>
        <Table>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.linkClicks.url)}</Table.Th>
              <Table.Th>{t(($) => $.linkClicks.clicks)}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {links.map((link) => (
              <Table.Tr key={link.url}>
                <Table.Td>
                  <Text size="sm" lineClamp={1}>
                    {link.url}
                  </Text>
                </Table.Td>
                <Table.Td>{link.clicks}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}
