import {
  Button,
  Checkbox,
  CloseButton,
  Divider,
  Group,
  Modal,
  NumberInput,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { TimeInput } from '@mantine/dates'
import { useDisclosure } from '@mantine/hooks'
import { useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import {
  siteWorkspacesGetSendingRulesOptions,
  siteWorkspacesGetSendingRulesQueryKey,
  siteWorkspacesReplaceSendingRulesMutation,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type {
  FrequencyCap,
  FrequencyCapChannel,
  SendingWindow,
  SiteSendingRulesResource,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import {
  zFrequencyCapMaxMessages,
  zFrequencyCapPeriodMinutes,
} from '../../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../../hooks/useResourceMutation.ts'
import { useWeekdayNames } from '../../../hooks/useWeekdayNames.ts'
import { useSchemaForm } from '../../../resources/schemaForm.ts'
import { hasErrorStatus } from '../../../utils/apiErrors.ts'
import { can } from '../../../utils/permissions.ts'
import {
  CAP_CHANNELS,
  clock,
  newCap,
  newWindow,
  periodIn,
  sendingRulesSchema,
  takesCountry,
  toValues,
  WEEKDAYS,
} from './sendingRules.ts'

function useChannelNames(): Record<FrequencyCapChannel, string> {
  const { t } = useTranslation()
  return {
    email: t(($) => $.settings.sendingRules.channels.email),
    telegram: t(($) => $.settings.sendingRules.channels.telegram),
    sms: t(($) => $.settings.sendingRules.channels.sms),
  }
}

// Where a rule applies: the Channel, or the Channel in one country.
function useRuleScope() {
  const { t } = useTranslation()
  const channels = useChannelNames()
  return (channel: FrequencyCapChannel, country: string | null) =>
    country === null
      ? channels[channel]
      : t(($) => $.settings.sendingRules.inCountry, { channel: channels[channel], country })
}

function CapLine({ cap }: { cap: FrequencyCap }) {
  const { t } = useTranslation()
  const scope = useRuleScope()
  if (cap.maxMessages === null || cap.periodMinutes === null) {
    return (
      <Text size="sm">
        {t(($) => $.settings.sendingRules.noCap, { scope: scope(cap.channel, cap.country) })}
      </Text>
    )
  }
  const period = periodIn(cap.periodMinutes)
  const periods = {
    days: t(($) => $.settings.sendingRules.periodDays, { count: period.count }),
    hours: t(($) => $.settings.sendingRules.periodHours, { count: period.count }),
    minutes: t(($) => $.settings.sendingRules.periodMinutes, { count: period.count }),
  }
  return (
    <Text size="sm">
      {t(($) => $.settings.sendingRules.capLine, {
        scope: scope(cap.channel, cap.country),
        max: cap.maxMessages,
        period: periods[period.unit],
      })}
    </Text>
  )
}

function WindowLine({ window }: { window: SendingWindow }) {
  const { t } = useTranslation()
  const scope = useRuleScope()
  const days = useWeekdayNames()
  const hours = window.hours
    .map((h) =>
      t(($) => $.settings.sendingRules.dayHours, {
        day: days[h.day],
        opens: clock(h.opensAt),
        closes: clock(h.closesAt),
      }),
    )
    .join(', ')
  return (
    <Text size="sm">
      {t(($) => $.settings.sendingRules.windowLine, {
        scope: scope(window.channel, window.country),
        hours,
      })}
    </Text>
  )
}

function SendingRulesForm({
  workspaceId,
  rules,
  onClose,
}: {
  workspaceId: string
  rules: SiteSendingRulesResource
  onClose: () => void
}) {
  const { t } = useTranslation()
  const channels = useChannelNames()
  const days = useWeekdayNames()
  const schema = sendingRulesSchema({
    countryOffPhone: t(($) => $.settings.sendingRules.countryOffPhone),
    capIncomplete: t(($) => $.settings.sendingRules.capIncomplete),
    neverOpens: t(($) => $.settings.sendingRules.neverOpens),
    closesAfterOpens: t(($) => $.settings.sendingRules.closesAfterOpens),
    timeRequired: t(($) => $.settings.sendingRules.timeRequired),
  })
  const queryClient = useQueryClient()
  const form = useSchemaForm(schema, toValues(rules))
  const saving = useResourceMutation({
    mutation: siteWorkspacesReplaceSendingRulesMutation(),
    errorTitle: t(($) => $.settings.sendingRules.saveError),
    successMessage: t(($) => $.settings.sendingRules.saved),
    // A refusal of a field shows under it; any other is the toast. When the rules changed since
    // they were read, they are read again: the form keeps what was typed, and a second Save sends
    // it on the revision read.
    handleError: async (error) => {
      if (hasErrorStatus(error, [409])) {
        await queryClient.invalidateQueries({
          queryKey: siteWorkspacesGetSendingRulesQueryKey({ path: { workspaceId } }),
        })
        return false
      }
      return form.showFieldErrors(error) === undefined
    },
    onDone: onClose,
  })
  const channelOptions = CAP_CHANNELS.map((channel) => ({
    value: channel,
    label: channels[channel],
  }))

  return (
    <form
      onSubmit={form.onSubmit((body) =>
        saving.mutate({
          path: { workspaceId },
          body: { ...body, revision: rules.revision },
        }),
      )}
    >
      <Stack>
        <Title order={5}>{t(($) => $.settings.sendingRules.capsTitle)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.settings.sendingRules.capsDescription)}
        </Text>
        {form.values.frequencyCaps.map((cap, i) => (
          <Group key={form.key(`frequencyCaps.${i}`)} align="flex-start" wrap="wrap">
            <Select
              label={t(($) => $.settings.sendingRules.channelLabel)}
              data={channelOptions}
              allowDeselect={false}
              w={140}
              {...form.getInputProps(`frequencyCaps.${i}.channel`)}
            />
            <TextInput
              label={t(($) => $.settings.sendingRules.countryLabel)}
              placeholder={t(($) => $.settings.sendingRules.everyCountry)}
              disabled={!takesCountry(cap.channel)}
              w={140}
              {...form.getInputProps(`frequencyCaps.${i}.country`)}
            />
            <NumberInput
              label={t(($) => $.settings.sendingRules.maxMessagesLabel)}
              placeholder={t(($) => $.settings.sendingRules.noCapPlaceholder)}
              allowDecimal={false}
              allowNegative={false}
              min={zFrequencyCapMaxMessages.minValue ?? undefined}
              max={zFrequencyCapMaxMessages.maxValue ?? undefined}
              clampBehavior="none"
              w={140}
              {...form.getInputProps(`frequencyCaps.${i}.maxMessages`)}
            />
            <NumberInput
              label={t(($) => $.settings.sendingRules.periodLabel)}
              description={t(($) => $.settings.sendingRules.periodDescription)}
              allowDecimal={false}
              allowNegative={false}
              min={zFrequencyCapPeriodMinutes.minValue ?? undefined}
              max={zFrequencyCapPeriodMinutes.maxValue ?? undefined}
              clampBehavior="none"
              w={180}
              {...form.getInputProps(`frequencyCaps.${i}.periodMinutes`)}
            />
            <CloseButton
              mt="xl"
              aria-label={t(($) => $.settings.sendingRules.removeCap)}
              onClick={() => form.removeListItem('frequencyCaps', i)}
            />
          </Group>
        ))}
        <Group>
          <Button variant="default" onClick={() => form.insertListItem('frequencyCaps', newCap())}>
            {t(($) => $.settings.sendingRules.addCap)}
          </Button>
        </Group>
        <Divider />
        <Title order={5}>{t(($) => $.settings.sendingRules.windowsTitle)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.settings.sendingRules.windowsDescription)}
        </Text>
        {form.values.sendingWindows.map((_, i) => (
          <Stack key={form.key(`sendingWindows.${i}`)} gap="xs">
            <Group align="flex-end">
              <TextInput
                label={t(($) => $.settings.sendingRules.countryLabel)}
                placeholder={t(($) => $.settings.sendingRules.everyCountry)}
                w={180}
                {...form.getInputProps(`sendingWindows.${i}.country`)}
              />
              <CloseButton
                mb={4}
                aria-label={t(($) => $.settings.sendingRules.removeWindow)}
                onClick={() => form.removeListItem('sendingWindows', i)}
              />
            </Group>
            {WEEKDAYS.map((day) => (
              <Group key={day} align="flex-start">
                <Checkbox
                  label={days[day]}
                  w={120}
                  mt="xs"
                  {...form.getInputProps(`sendingWindows.${i}.days.${day}.open`, {
                    type: 'checkbox',
                  })}
                />
                <TimeInput
                  aria-label={t(($) => $.settings.sendingRules.opensLabel, { day: days[day] })}
                  w={120}
                  {...form.getInputProps(`sendingWindows.${i}.days.${day}.opensAt`)}
                />
                <TimeInput
                  aria-label={t(($) => $.settings.sendingRules.closesLabel, { day: days[day] })}
                  w={120}
                  {...form.getInputProps(`sendingWindows.${i}.days.${day}.closesAt`)}
                />
              </Group>
            ))}
            <Divider variant="dashed" />
          </Stack>
        ))}
        <Group>
          <Button
            variant="default"
            onClick={() => form.insertListItem('sendingWindows', newWindow())}
          >
            {t(($) => $.settings.sendingRules.addWindow)}
          </Button>
        </Group>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={saving.isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

function SendingRules({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const { data } = useSuspenseQuery(
    siteWorkspacesGetSendingRulesOptions({ path: { workspaceId: workspace.id } }),
  )
  return (
    <Stack gap="xs">
      <Text size="sm" fw={500}>
        {t(($) => $.settings.sendingRules.capsTitle)}
      </Text>
      {data.frequencyCaps.length === 0 ? (
        <Text size="sm">{t(($) => $.settings.sendingRules.noCaps)}</Text>
      ) : (
        data.frequencyCaps.map((cap) => <CapLine key={`${cap.channel}-${cap.country}`} cap={cap} />)
      )}
      <Text size="sm" fw={500}>
        {t(($) => $.settings.sendingRules.windowsTitle)}
      </Text>
      {data.sendingWindows.length === 0 ? (
        <Text size="sm">{t(($) => $.settings.sendingRules.noWindows)}</Text>
      ) : (
        data.sendingWindows.map((window) => (
          <WindowLine key={`${window.channel}-${window.country}`} window={window} />
        ))
      )}
      {/* Only an Owner or Admin changes them (ADR 0137); every member reads them. */}
      {can(workspace, 'workspace:update') ? (
        <Group>
          <Button variant="light" onClick={open}>
            {t(($) => $.settings.sendingRules.change)}
          </Button>
        </Group>
      ) : null}
      <Modal
        opened={opened}
        onClose={close}
        title={t(($) => $.settings.sendingRules.formTitle)}
        size="xl"
      >
        <SendingRulesForm workspaceId={workspace.id} rules={data} onClose={close} />
      </Modal>
    </Stack>
  )
}

// SendingRulesSection holds the Workspace's Frequency caps and Sending windows (ADR 0134, ADR 0137)
// as text, with a Change action that opens their form. It says that the Workspace answers for its
// compliance: the defaults are not legal advice.
export function SendingRulesSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.sendingRules.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.sendingRules.compliance)}
      </Text>
      <QueryBoundary
        fallback={<BlockSkeleton />}
        errorTitle={t(($) => $.settings.sendingRules.loadError)}
      >
        <SendingRules workspace={workspace} />
      </QueryBoundary>
    </Stack>
  )
}
