import { Anchor, Box, Button, Code, Group, Stack, Table, Tabs, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteCapturedMessagesDeleteMutation,
  siteCapturedMessagesGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as capturedMessageRoute } from '../../routes/workspaces/$workspaceId/captured-messages/$messageId/index.tsx'
import { Route as capturedMessagesRoute } from '../../routes/workspaces/$workspaceId/captured-messages/index.tsx'

export function CapturedMessagePage() {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={<PageSkeleton />}
      errorTitle={t(($) => $.capturedMessages.messageLoadError)}
    >
      <CapturedMessage />
    </QueryBoundary>
  )
}

function CapturedMessage() {
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  const { workspaceId, messageId } = capturedMessageRoute.useParams()
  const navigate = capturedMessageRoute.useNavigate()
  const confirm = useConfirmation()

  const message = useSuspenseQuery(
    siteCapturedMessagesGetOptions({ path: { workspaceId, id: messageId } }),
  ).data
  const deleteMessage = useResourceMutation({
    mutation: siteCapturedMessagesDeleteMutation(),
    successMessage: t(($) => $.capturedMessages.deleted),
    errorTitle: t(($) => $.capturedMessages.deleteError),
    onDone: async () => navigate({ to: capturedMessagesRoute.to, params: { workspaceId } }),
  })

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{message.subject}</Title>
        <Group>
          <ButtonLink variant="default" to={capturedMessagesRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
          <Button
            color="red"
            variant="light"
            loading={deleteMessage.isPending}
            onClick={() => {
              confirm({
                onConfirm: () => deleteMessage.mutate({ path: { workspaceId, id: message.id } }),
              })
            }}
          >
            {t(($) => $.actions.delete)}
          </Button>
        </Group>
      </Group>
      <Text c="dimmed" size="sm">
        {t(($) => $.capturedMessages.envelope, {
          from: message.fromName ? `${message.fromName} <${message.from}>` : message.from,
          recipient: message.recipient,
          at: formatDateTime(message.createdAt),
        })}
      </Text>

      <Tabs defaultValue="rendered">
        <Tabs.List>
          <Tabs.Tab value="rendered">{t(($) => $.capturedMessages.tabs.rendered)}</Tabs.Tab>
          <Tabs.Tab value="html">{t(($) => $.capturedMessages.tabs.html)}</Tabs.Tab>
          <Tabs.Tab value="text">{t(($) => $.capturedMessages.tabs.text)}</Tabs.Tab>
          <Tabs.Tab value="headers">{t(($) => $.capturedMessages.tabs.headers)}</Tabs.Tab>
          <Tabs.Tab value="links">{t(($) => $.capturedMessages.tabs.links)}</Tabs.Tab>
        </Tabs.List>

        <Tabs.Panel value="rendered" pt="md">
          {/* sandbox="" grants nothing: no scripts, no same-origin. Images still load, so opening
              the message is a real open. */}
          <Box
            component="iframe"
            title={t(($) => $.capturedMessages.renderedTitle)}
            srcDoc={message.html}
            sandbox=""
            w="100%"
            h={600}
            bg="white"
            bd="1px solid var(--mantine-color-default-border)"
          />
        </Tabs.Panel>
        <Tabs.Panel value="html" pt="md">
          <Code block>{message.html}</Code>
        </Tabs.Panel>
        <Tabs.Panel value="text" pt="md">
          <Code block>{message.text}</Code>
        </Tabs.Panel>
        <Tabs.Panel value="headers" pt="md">
          <Table withTableBorder>
            <Table.Tbody>
              {message.headers.map((header, index) => (
                <Table.Tr key={`${header.name}-${index}`}>
                  <Table.Td fw={600}>{header.name}</Table.Td>
                  <Table.Td>{header.value}</Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Tabs.Panel>
        <Tabs.Panel value="links" pt="md">
          <Stack gap="xs">
            {message.links.length === 0 ? (
              <Text c="dimmed">{t(($) => $.capturedMessages.noLinks)}</Text>
            ) : null}
            {message.links.map((link, index) => (
              <Anchor key={`${link}-${index}`} href={link} target="_blank" rel="noopener" size="sm">
                {link}
              </Anchor>
            ))}
          </Stack>
        </Tabs.Panel>
      </Tabs>
    </Stack>
  )
}
