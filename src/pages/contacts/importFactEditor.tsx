import {
  ActionIcon,
  Button,
  Group,
  Select,
  SimpleGrid,
  Stack,
  TagsInput,
  TextInput,
} from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type {
  ContactImportConsentEvidenceInput,
  ContactImportRefusal,
  ContactImportRefusalChannel,
} from '../../generated/site/types.gen.ts'
import {
  zContactImportRefusal,
  zContactImportRefusalChannel,
} from '../../generated/site/zod.gen.ts'
import { consent, engagementTracking, refusal, time, timesFacts } from './importFacts.ts'
import type { FactSettings } from './importFacts.ts'

interface Props {
  // The kind of the column: refusal, consent, engagementTracking or time.
  kind: string
  // The column's name in the file, which labels its controls.
  column: string
  fact: FactSettings
  // The file's header and what each column imports as, to name the column a time times and the
  // columns evidence is read from.
  header: string[]
  targets: string[]
  // Whether the Member holds suppressions:manage, without which a Suppression is not offered.
  canSuppress: boolean
  onChange: (fact: FactSettings) => void
}

// The settings of a column of refusals, consent, Engagement tracking or their times (ADR 0138):
// the Channel, the values that mean each fact, a consent's evidence and the column a time times.
export function FactColumnEditor({
  kind,
  column,
  fact,
  header,
  targets,
  canSuppress,
  onChange,
}: Props) {
  const { t } = useTranslation()
  const set = (patch: Partial<FactSettings>) => onChange({ ...fact, ...patch })
  const columnName = (at: number) => (header[at] === '' ? `#${at + 1}` : (header[at] ?? ''))
  const columns = header.map((_, at) => ({ value: String(at), label: columnName(at) }))

  if (kind === time) {
    return (
      <Select
        label={t(($) => $.contactImports.facts.timeOf)}
        description={t(($) => $.contactImports.facts.timeOfHint)}
        aria-label={t(($) => $.contactImports.facts.timeOfColumn, { column })}
        placeholder={t(($) => $.contactImports.facts.chooseColumn)}
        data={columns.filter((_, at) => timesFacts(targets[at] ?? ''))}
        value={fact.of === null ? null : String(fact.of)}
        onChange={(at) => set({ of: at === null ? null : Number(at) })}
      />
    )
  }

  const channelLabels: Record<ContactImportRefusalChannel, string> = {
    email: t(($) => $.contactImports.facts.channels.email),
    sms: t(($) => $.contactImports.facts.channels.sms),
    telegram: t(($) => $.contactImports.facts.channels.telegram),
  }
  const channels = zContactImportRefusalChannel.options
    .filter((channel) => kind === refusal || channel !== 'telegram')
    .map((channel) => ({ value: channel, label: channelLabels[channel] }))
  const channelSelect = (
    <Select
      label={t(($) => $.contactImports.facts.channel)}
      aria-label={t(($) => $.contactImports.facts.channelColumn, { column })}
      data={channels}
      value={fact.channel}
      allowDeselect={false}
      onChange={(channel) => set({ channel: zContactImportRefusalChannel.parse(channel) })}
    />
  )

  if (kind === refusal) {
    const refusalLabels: Record<ContactImportRefusal, string> = {
      unsubscribeEverything: t(($) => $.contactImports.facts.refusals.unsubscribeEverything),
      unsubscribeBroadcasts: t(($) => $.contactImports.facts.refusals.unsubscribeBroadcasts),
      suppression: t(($) => $.contactImports.facts.refusals.suppression),
    }
    const refusals = zContactImportRefusal.options.map((value) => ({
      value,
      label: refusalLabels[value],
      disabled:
        (value === 'suppression' && !canSuppress) ||
        (value === 'unsubscribeBroadcasts' && fact.channel === 'sms'),
    }))
    const setValue = (at: number, patch: Partial<FactSettings['refusals'][number]>) =>
      set({ refusals: fact.refusals.map((v, i) => (i === at ? { ...v, ...patch } : v)) })
    return (
      <Stack gap="xs">
        {channelSelect}
        {fact.refusals.map((v, at) => (
          <Group key={at} align="end" wrap="nowrap">
            <TextInput
              flex={1}
              label={at === 0 ? t(($) => $.contactImports.facts.value) : undefined}
              aria-label={t(($) => $.contactImports.facts.valueOf, { column, at: at + 1 })}
              placeholder={t(($) => $.contactImports.facts.valuePlaceholder)}
              value={v.value}
              onChange={(event) => setValue(at, { value: event.currentTarget.value })}
            />
            <Select
              flex={1}
              label={at === 0 ? t(($) => $.contactImports.facts.records) : undefined}
              aria-label={t(($) => $.contactImports.facts.recordsOf, { column, at: at + 1 })}
              data={refusals}
              value={v.refusal}
              allowDeselect={false}
              onChange={(r) => setValue(at, { refusal: zContactImportRefusal.parse(r) })}
            />
            <ActionIcon
              variant="subtle"
              color="gray"
              aria-label={t(($) => $.contactImports.facts.removeValue)}
              disabled={fact.refusals.length === 1}
              onClick={() => set({ refusals: fact.refusals.filter((_, i) => i !== at) })}
            >
              <IconTrash size={16} />
            </ActionIcon>
          </Group>
        ))}
        <Group>
          <Button
            variant="default"
            size="xs"
            onClick={() =>
              set({ refusals: [...fact.refusals, { value: '', refusal: 'unsubscribeEverything' }] })
            }
          >
            {t(($) => $.contactImports.facts.addValue)}
          </Button>
        </Group>
      </Stack>
    )
  }

  if (kind === engagementTracking) {
    return (
      <SimpleGrid cols={{ base: 1, sm: 3 }}>
        {channelSelect}
        <TagsInput
          label={t(($) => $.contactImports.facts.granted)}
          value={fact.values}
          onChange={(values) => set({ values })}
        />
        <TagsInput
          label={t(($) => $.contactImports.facts.denied)}
          value={fact.denied}
          onChange={(denied) => set({ denied })}
        />
      </SimpleGrid>
    )
  }

  if (kind !== consent) {
    return null
  }
  const ev = fact.evidence
  const setEvidence = (patch: Partial<ContactImportConsentEvidenceInput>) =>
    set({ evidence: { ...ev, ...patch } })
  // A part of the evidence: its text given once, or the column that holds it on each row.
  const part = (name: 'wording' | 'source' | 'brand', label: string) => {
    const from = ev[`${name}Column`]
    return (
      <Group grow align="end">
        <TextInput
          label={label}
          value={ev[name] ?? ''}
          disabled={from !== null}
          onChange={(event) =>
            setEvidence({ [name]: event.currentTarget.value, [`${name}Column`]: null })
          }
        />
        <Select
          label={t(($) => $.contactImports.facts.fromColumn)}
          placeholder={t(($) => $.contactImports.facts.givenOnce)}
          data={columns}
          value={from === null ? null : String(from)}
          clearable
          onChange={(at) =>
            setEvidence({ [name]: null, [`${name}Column`]: at === null ? null : Number(at) })
          }
        />
      </Group>
    )
  }
  return (
    <Stack gap="xs">
      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        {channelSelect}
        <TagsInput
          label={t(($) => $.contactImports.facts.consentValues)}
          value={fact.values}
          onChange={(values) => set({ values })}
        />
      </SimpleGrid>
      {part(
        'wording',
        t(($) => $.contactImports.facts.wording),
      )}
      {part(
        'source',
        t(($) => $.contactImports.facts.source),
      )}
      {part(
        'brand',
        t(($) => $.contactImports.facts.brand),
      )}
    </Stack>
  )
}
