import { Button, Code, CopyButton, Group, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteSendingDomainsListOptions,
  siteSendingDomainsListQueryKey,
  siteSendingDomainsVerifyMutation,
  siteSetupChecklistGetQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

// SetupDomainStep is the body of a pending Sending domain step: the DKIM record the person
// must publish, with a copy button, and a "check now" action that runs the existing domain
// verification and refreshes the checklist so the step turns done without a reload.
export function SetupDomainStep({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const domainsQuery = useQuery(siteSendingDomainsListOptions({ path: { workspaceId } }))
  const waiting = domainsQuery.data?.items.find((d) => !d.verified)

  const checkNow = useResourceMutation({
    mutation: siteSendingDomainsVerifyMutation(),
    invalidate: [
      siteSendingDomainsListQueryKey({ path: { workspaceId } }),
      siteSetupChecklistGetQueryKey({ path: { workspaceId } }),
    ],
    errorTitle: t(($) => $.setupChecklist.checkError),
  })

  if (!waiting) {
    return null
  }
  const { dkimRecord } = waiting

  return (
    <Stack gap="xs" mt="xs">
      <Text size="xs" c="dimmed">
        {t(($) => $.setupChecklist.recordHint, { domain: waiting.domain })}
      </Text>
      <Group gap="xs" wrap="wrap">
        <Text size="xs" fw={600}>
          {dkimRecord.type}
        </Text>
        <Code>{dkimRecord.host}</Code>
      </Group>
      <Group gap="xs" wrap="nowrap" align="flex-start">
        <Code block maw={420}>
          {dkimRecord.value}
        </Code>
        <CopyButton value={dkimRecord.value}>
          {({ copied, copy }) => (
            <Button
              size="compact-xs"
              variant="light"
              aria-label={t(($) => $.setupChecklist.copyDkim)}
              onClick={copy}
            >
              {copied ? t(($) => $.setupChecklist.copied) : t(($) => $.setupChecklist.copy)}
            </Button>
          )}
        </CopyButton>
      </Group>
      <Group>
        <Button
          size="compact-sm"
          variant="light"
          loading={checkNow.isPending}
          onClick={() => checkNow.mutate({ path: { workspaceId, id: waiting.id } })}
        >
          {t(($) => $.setupChecklist.checkNow)}
        </Button>
      </Group>
    </Stack>
  )
}
