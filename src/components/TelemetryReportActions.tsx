import { Alert, Anchor, Button, Code, Group, Loader, Modal, Stack, Text } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteInstancePreviewTelemetryReportOptions } from '../generated/site/@tanstack/react-query.gen.ts'

// The page that documents the Telemetry report's schema (ADR 0032).
const telemetrySchemaDocsUrl = 'https://docs.getsphericon.app/operations/telemetry-schema'

// TelemetryReportActions lets an Owner read what the Instance sends: a link to the published
// schema and a preview of the report as it would be sent right now, whether or not sending
// is on. The report is fetched only once the preview is opened.
export function TelemetryReportActions({ size = 'sm' }: { size?: 'xs' | 'sm' }) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const report = useQuery({
    ...siteInstancePreviewTelemetryReportOptions(),
    enabled: opened,
    retry: false,
  })

  return (
    <>
      <Group gap="md">
        <Anchor href={telemetrySchemaDocsUrl} target="_blank" rel="noopener noreferrer" size={size}>
          {t(($) => $.instance.report.schema)}
        </Anchor>
        <Button variant="subtle" size={size === 'xs' ? 'compact-xs' : 'compact-sm'} onClick={open}>
          {t(($) => $.instance.report.preview)}
        </Button>
      </Group>
      <Modal
        opened={opened}
        onClose={close}
        size="lg"
        title={t(($) => $.instance.report.previewTitle)}
      >
        <Stack gap="sm">
          <Text size="sm" c="dimmed">
            {t(($) => $.instance.report.previewDescription)}
          </Text>
          {report.isPending ? <Loader size="sm" /> : null}
          {report.isError ? (
            <Alert color="red">{t(($) => $.instance.report.loadError)}</Alert>
          ) : null}
          {report.data ? <Code block>{JSON.stringify(report.data, null, 2)}</Code> : null}
        </Stack>
      </Modal>
    </>
  )
}
