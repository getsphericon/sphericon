import {
  Button,
  Checkbox,
  CloseButton,
  Divider,
  Group,
  NumberInput,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { TimeInput } from '@mantine/dates'
import { useTranslation } from 'react-i18next'

import {
  zFrequencyCapMaxMessages,
  zFrequencyCapPeriodMinutes,
} from '../../../generated/site/zod.gen.ts'
import { useWeekdayNames } from '../../../hooks/useWeekdayNames.ts'
import type { ResourceFormProps } from '../../../resources/defineResource.tsx'
import { takesCountry } from '../../../utils/sendingRules.ts'
import { useChannelNames } from './sendingRuleNames.ts'
import { CAP_CHANNELS, newCap, newWindow, WEEKDAYS } from './sendingRules.ts'
import type { SendingRulesPayload, SendingRulesValues } from './sendingRules.ts'

// SendingRulesFields is the form of the Workspace's Frequency caps and Sending windows (ADR 0134,
// ADR 0137), in the editor session of sendingRulesEditor: a refusal of a field shows under it, and
// the rules changed since they were read are the session's changed notice.
export function SendingRulesFields({
  form,
  isPending,
  onSubmit,
  onCancel,
}: ResourceFormProps<SendingRulesPayload, SendingRulesValues>) {
  const { t } = useTranslation()
  const channels = useChannelNames()
  const days = useWeekdayNames()
  const channelOptions = CAP_CHANNELS.map((channel) => ({
    value: channel,
    label: channels[channel],
  }))

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
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
          <Button variant="default" onClick={onCancel}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
