import { Button, Group, Menu, Stack, Table, Text, Title } from '@mantine/core'
import { useClipboard } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteIntegrationsListOptions,
  siteTelegramIssueStartLinkMutation,
  siteTelegramListBindingsOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

interface Props {
  workspaceId: string
  contactId: string
}

// The Contact's Telegram chats (a Binding is made only by what the bot receives, so the card
// reads them and never edits them) and the action that issues the start link a person opens to
// bind theirs. With several bots the action offers each of them.
export function TelegramSection({ workspaceId, contactId }: Props) {
  const { t } = useTranslation()
  const { formatDateTime } = useDateFormat()
  const clipboard = useClipboard()
  const bindings = useQuery(siteTelegramListBindingsOptions({ path: { workspaceId, contactId } }))
  const integrations = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
  )
  const bots = (integrations.data ?? []).filter((i) => i.channel === 'telegram' && i.enabled)
  const chats = bindings.data ?? []

  const issue = useResourceMutation({
    mutation: siteTelegramIssueStartLinkMutation(),
    invalidate: false,
    successMessage: t(($) => $.contacts.telegramLinkCopied),
    errorTitle: t(($) => $.contacts.telegramLinkErrorTitle),
    onDone: (link) => clipboard.copy(link.url),
  })
  const copyLink = (integrationId: string) =>
    issue.mutate({ path: { workspaceId, contactId }, body: { integrationId } })

  if (chats.length === 0 && bots.length === 0) {
    return null
  }

  return (
    <Stack>
      <Group justify="space-between">
        <Title order={5}>{t(($) => $.contacts.telegramTitle)}</Title>
        {bots.length === 1 ? (
          <Button
            variant="default"
            loading={issue.isPending}
            onClick={() => copyLink(bots[0]?.id ?? '')}
          >
            {t(($) => $.contacts.telegramCopyLink)}
          </Button>
        ) : null}
        {bots.length > 1 ? (
          <Menu>
            <Menu.Target>
              <Button variant="default" loading={issue.isPending}>
                {t(($) => $.contacts.telegramCopyLink)}
              </Button>
            </Menu.Target>
            <Menu.Dropdown>
              {bots.map((bot) => (
                <Menu.Item key={bot.id} onClick={() => copyLink(bot.id)}>
                  {bot.name}
                </Menu.Item>
              ))}
            </Menu.Dropdown>
          </Menu>
        ) : null}
      </Group>
      {chats.length === 0 ? (
        <Text c="dimmed" size="sm">
          {t(($) => $.contacts.telegramNoChats)}
        </Text>
      ) : (
        <Table withTableBorder>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.contacts.telegramBot)}</Table.Th>
              <Table.Th>{t(($) => $.contacts.telegramBoundSince)}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {chats.map((chat) => (
              <Table.Tr key={chat.id}>
                <Table.Td>
                  {chat.botUsername ? `@${chat.botUsername}` : chat.integrationName}
                </Table.Td>
                <Table.Td>{formatDateTime(chat.boundAt)}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Stack>
  )
}
