import type {
  ContactImportColumn,
  ContactImportColumnInput,
  ContactImportConsentEvidenceInput,
  ContactImportDateFormat,
  ContactImportRefusalChannel,
  ContactImportRefusalValueInput,
} from '../../generated/site/types.gen.ts'

// The columns of a CSV import that bring the old platform's refusals and the consent a Workspace
// collected, each with the time it occurred (ADR 0138): their kinds, as the value of a column's
// type control, and their settings.
export const refusal = 'refusal'
export const consent = 'consent'
export const engagementTracking = 'engagementTracking'
export const time = 'time'

export const factTargets: readonly string[] = [refusal, consent, engagementTracking, time]

// Whether a column of target states a fact a time column may time.
export function timesFacts(target: string): boolean {
  return target === refusal || target === consent || target === engagementTracking
}

// What a refusal, consent, tracking or time column records, as the mapping form holds it. A
// refusal reads `channel` and `refusals`; a consent `channel` (email or sms), `values` and
// `evidence`; a tracking column `channel`, `values` (granted) and `denied`; a time column `of`.
export interface FactSettings {
  channel: ContactImportRefusalChannel
  refusals: ContactImportRefusalValueInput[]
  values: string[]
  denied: string[]
  evidence: ContactImportConsentEvidenceInput
  of: number | null
}

// The settings a column starts with when it is first set to a fact kind.
export function emptyFact(): FactSettings {
  return {
    channel: 'email',
    refusals: [{ value: '', refusal: 'unsubscribeEverything' }],
    values: [],
    denied: [],
    evidence: {
      wording: '',
      wordingColumn: null,
      source: '',
      sourceColumn: null,
      brand: '',
      brandColumn: null,
    },
    of: null,
  }
}

// The settings of a column the server answered, undefined for a column of another kind.
export function factOf(column: ContactImportColumn): FactSettings | undefined {
  const fact = emptyFact()
  if (column.kind === 'refusal') {
    return { ...fact, channel: column.channel, refusals: column.values }
  }
  if (column.kind === 'consent') {
    return { ...fact, channel: column.channel, values: column.values, evidence: column.evidence }
  }
  if (column.kind === 'engagementTracking') {
    return { ...fact, channel: column.channel, values: column.granted, denied: column.denied }
  }
  return column.kind === 'time' ? { ...fact, of: column.of } : undefined
}

// The channel of a consent or tracking column: email or sms, email for a refusal's telegram.
function consentChannel(fact: FactSettings): 'email' | 'sms' {
  return fact.channel === 'sms' ? 'sms' : 'email'
}

// A text given once, null when blank (a column holds it then).
function given(text: string | null): string | null {
  const trimmed = text?.trim() ?? ''
  return trimmed === '' ? null : trimmed
}

// The column a request saves for a fact kind and its settings; a time column takes the date
// format the column's control chose. Undefined for any other kind.
export function factColumnOf(
  kind: string,
  fact: FactSettings,
  dateFormat: ContactImportDateFormat | null,
): ContactImportColumnInput | undefined {
  const values = fact.values.map((v) => v.trim()).filter((v) => v !== '')
  switch (kind) {
    case refusal:
      return {
        kind: 'refusal',
        channel: fact.channel,
        values: fact.refusals
          .map((v) => ({ ...v, value: v.value.trim() }))
          .filter((v) => v.value !== ''),
      }
    case consent: {
      const ev = fact.evidence
      return {
        kind: 'consent',
        channel: consentChannel(fact),
        values,
        evidence: {
          wording: ev.wordingColumn === null ? given(ev.wording) : null,
          wordingColumn: ev.wordingColumn,
          source: ev.sourceColumn === null ? given(ev.source) : null,
          sourceColumn: ev.sourceColumn,
          brand: ev.brandColumn === null ? given(ev.brand) : null,
          brandColumn: ev.brandColumn,
        },
      }
    }
    case engagementTracking:
      return {
        kind: 'engagementTracking',
        channel: consentChannel(fact),
        granted: values,
        denied: fact.denied.map((v) => v.trim()).filter((v) => v !== ''),
      }
    case time:
      return fact.of === null || dateFormat === null
        ? undefined
        : { kind: 'time', of: fact.of, dateFormat }
    default:
      return undefined
  }
}

// Whether the settings record a Suppression, which needs suppressions:manage.
export function recordsSuppression(kind: string, fact: FactSettings | undefined): boolean {
  return kind === refusal && (fact?.refusals.some((v) => v.refusal === 'suppression') ?? false)
}
