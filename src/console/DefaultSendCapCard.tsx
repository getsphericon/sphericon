import { Alert, Button, Card, Group, Modal, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { IconAlertTriangle } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  staffSendCapDefaultsClearMutation,
  staffSendCapDefaultsGetOptions,
  staffSendCapDefaultsSetMutation,
} from '../generated/staffee/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { SendCapForm } from './SendCapCard.tsx'
import { useSendCapNotice } from './useSendCapNotice.ts'

// The Instance's default Send cap (ADR 0067): what every Workspace created from now on starts
// with. While none is set, new Workspaces send uncapped, so the card warns. An Instance without
// Send caps shows nothing.
export function DefaultSendCapCard() {
  const { t, i18n } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const notice = useSendCapNotice()
  const query = useQuery(staffSendCapDefaultsGetOptions())
  const set = useResourceMutation({
    mutation: staffSendCapDefaultsSetMutation(),
    errorTitle: t(($) => $.console.sendCapDefault.error),
    onDone: (change) => {
      notice(change)
      close()
    },
  })
  const clear = useResourceMutation({
    mutation: staffSendCapDefaultsClearMutation(),
    errorTitle: t(($) => $.console.sendCapDefault.error),
    onDone: notice,
  })

  if (!query.data?.sendCaps) {
    return null
  }
  const sendCap = query.data.sendCap

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        {t(($) => $.console.sendCapDefault.title)}
      </Title>
      {sendCap === null ? (
        <Alert color="orange" variant="light" icon={<IconAlertTriangle size={18} />}>
          {t(($) => $.console.sendCapDefault.unset)}
        </Alert>
      ) : (
        <Text size="sm">
          {t(($) => $.console.workspace.sendCap.value, {
            count: sendCap,
            formatted: new Intl.NumberFormat(i18n.language).format(sendCap),
          })}
        </Text>
      )}
      <Text size="xs" c="dimmed" mt={4}>
        {t(($) => $.console.sendCapDefault.description)}
      </Text>
      <Group mt="md">
        <Button variant="light" onClick={open}>
          {sendCap === null
            ? t(($) => $.console.sendCapDefault.set)
            : t(($) => $.console.sendCapDefault.change)}
        </Button>
        {sendCap !== null && (
          <Button
            variant="subtle"
            color="gray"
            loading={clear.isPending}
            onClick={() =>
              void modals.openConfirmModal({
                title: t(($) => $.console.sendCapDefault.clearTitle),
                children: t(($) => $.console.sendCapDefault.clearDescription),
                labels: {
                  cancel: t(($) => $.console.workspace.suspend.cancel),
                  confirm: t(($) => $.console.sendCapDefault.clear),
                },
                onConfirm: () => clear.mutate({}),
              })
            }
          >
            {t(($) => $.console.sendCapDefault.clear)}
          </Button>
        )}
      </Group>
      <Modal opened={opened} onClose={close} title={t(($) => $.console.sendCapDefault.title)}>
        <SendCapForm
          sendCap={sendCap}
          description={t(($) => $.console.sendCapDefault.description)}
          pending={set.isPending}
          onSave={(body) => set.mutate({ body })}
          onClose={close}
        />
      </Modal>
    </Card>
  )
}
