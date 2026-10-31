import { Button, Card, Group, Modal, NumberInput, Stack, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  staffWorkspacesClearSendCapMutation,
  staffWorkspacesSetSendCapMutation,
} from '../generated/staffee/@tanstack/react-query.gen.ts'
import type { StaffSendCapChange } from '../generated/staffee/types.gen.ts'
import { zStaffSendCapInput } from '../generated/staffee/zod.gen.ts'
import { useNotify } from '../hooks/useNotify.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { useSchemaForm } from '../resources/schemaForm.ts'

// A NumberInput holds '' while empty, so the field reads that as missing before the contract's
// own bounds judge the number.
const sendCapSchema = z.object({
  sendCap: z
    .union([z.literal('').transform(() => undefined), z.number()])
    .pipe(zStaffSendCapInput.shape.sendCap),
})

function useSendCapNotice() {
  const { t } = useTranslation()
  const notify = useNotify()
  return (change: StaffSendCapChange) => {
    notify(
      change.changed
        ? {
            color: 'teal',
            title: t(($) => $.notifications.successTitle),
            message: t(($) => $.console.workspace.sendCap.done),
          }
        : {
            color: 'blue',
            title: t(($) => $.console.workspace.unchangedTitle),
            message: t(($) => $.console.workspace.sendCap.unchanged),
          },
    )
  }
}

function SendCapForm({
  workspaceId,
  sendCap,
  onClose,
}: {
  workspaceId: string
  sendCap: number | null | undefined
  onClose: () => void
}) {
  const { t } = useTranslation()
  const form = useSchemaForm(sendCapSchema, { sendCap: sendCap ?? '' })
  const notice = useSendCapNotice()
  const mutation = useResourceMutation({
    mutation: staffWorkspacesSetSendCapMutation(),
    errorTitle: t(($) => $.console.workspace.sendCap.error),
    onDone: (change) => {
      notice(change)
      onClose()
    },
  })

  return (
    <form onSubmit={form.onSubmit((body) => mutation.mutate({ path: { workspaceId }, body }))}>
      <Stack>
        <NumberInput
          label={t(($) => $.console.workspace.sendCap.label)}
          description={t(($) => $.console.workspace.sendCap.description)}
          allowDecimal={false}
          allowNegative={false}
          thousandSeparator=" "
          withAsterisk
          data-autofocus
          {...form.getInputProps('sendCap')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.console.workspace.suspend.cancel)}
          </Button>
          <Button type="submit" loading={mutation.isPending}>
            {t(($) => $.console.workspace.sendCap.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// The Workspace's Send cap (ADR 0067): at most this many Outbound messages in any 24 hours,
// beyond which marketing waits and Transactional still goes out. Only a saas instance has one.
export function SendCapCard({
  workspaceId,
  sendCap,
}: {
  workspaceId: string
  sendCap: number | null | undefined
}) {
  const { t, i18n } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const notice = useSendCapNotice()
  const clear = useResourceMutation({
    mutation: staffWorkspacesClearSendCapMutation(),
    errorTitle: t(($) => $.console.workspace.sendCap.error),
    onDone: notice,
  })
  const capped = sendCap !== null && sendCap !== undefined

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        {t(($) => $.console.workspace.sendCap.title)}
      </Title>
      {capped ? (
        <Text size="sm">
          {t(($) => $.console.workspace.sendCap.value, {
            count: sendCap,
            formatted: new Intl.NumberFormat(i18n.language).format(sendCap),
          })}
        </Text>
      ) : (
        <Text size="sm" c="dimmed">
          {t(($) => $.console.workspace.sendCap.none)}
        </Text>
      )}
      <Group mt="md">
        <Button variant="light" onClick={open}>
          {capped
            ? t(($) => $.console.workspace.sendCap.change)
            : t(($) => $.console.workspace.sendCap.set)}
        </Button>
        {capped && (
          <Button
            variant="subtle"
            color="gray"
            loading={clear.isPending}
            onClick={() =>
              void modals.openConfirmModal({
                title: t(($) => $.console.workspace.sendCap.clearTitle),
                children: t(($) => $.console.workspace.sendCap.clearDescription),
                labels: {
                  cancel: t(($) => $.console.workspace.suspend.cancel),
                  confirm: t(($) => $.console.workspace.sendCap.clear),
                },
                onConfirm: () => clear.mutate({ path: { workspaceId } }),
              })
            }
          >
            {t(($) => $.console.workspace.sendCap.clear)}
          </Button>
        )}
      </Group>
      <Modal opened={opened} onClose={close} title={t(($) => $.console.workspace.sendCap.title)}>
        <SendCapForm workspaceId={workspaceId} sendCap={sendCap} onClose={close} />
      </Modal>
    </Card>
  )
}
