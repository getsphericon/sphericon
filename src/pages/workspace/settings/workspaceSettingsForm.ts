import { createContext } from 'react'
import * as z from 'zod'

import {
  zRetentionDays,
  zSiteUpdateRevenueSettingsInput,
  zSiteUpdateWorkspaceInput,
  zSiteWorkspaceTimezoneInput,
} from '../../../generated/site/zod.gen.ts'
import { resourceFormSchema } from '../../../resources/resourceFormSchema.ts'
import type { ResourceFormSchema } from '../../../resources/resourceFormSchema.ts'

// Whether the person may change the settings section the form is in; its Save is offered only then.
export const CanEdit = createContext(false)

// What each settings form holds and sends. The revision is the editor session's to add
// (withRevision, ADR 0119), so the forms leave it out.
export interface GeneralValues {
  name: string
  postalAddress: string
}

export interface TimezoneValues {
  timezone: string
}

export interface RevenueValues {
  reportingCurrency: string | null
  attributionWindowDays: number
}

// A blank input holds '': no window, the log is kept forever.
export interface RetentionValues {
  retentionDays: number | ''
}

export interface RetentionPayload {
  retentionDays: number | null
}

const generalBody = zSiteUpdateWorkspaceInput.omit({ revision: true })
const timezoneBody = zSiteWorkspaceTimezoneInput.omit({ revision: true })
const revenueBody = zSiteUpdateRevenueSettingsInput.omit({ revision: true })

export type GeneralPayload = z.output<typeof generalBody>
export type TimezonePayload = z.output<typeof timezoneBody>
export type RevenuePayload = z.output<typeof revenueBody>

const generalBlank: GeneralValues = { name: '', postalAddress: '' }
const timezoneBlank: TimezoneValues = { timezone: '' }
// A Reporting currency not chosen yet (null) is left out of the save; the window always goes.
const revenueBlank: RevenueValues = { reportingCurrency: null, attributionWindowDays: 5 }

export const generalSchema = resourceFormSchema(generalBody, generalBlank)
export const timezoneSchema = resourceFormSchema(timezoneBody, timezoneBlank)
export const revenueSchema = resourceFormSchema(revenueBody, revenueBlank)

const retention = z
  .custom<RetentionValues>((value) => typeof value === 'object' && value !== null)
  .transform(({ retentionDays }) => ({
    retentionDays: retentionDays === '' ? null : retentionDays,
  }))
  .pipe(z.object({ retentionDays: zRetentionDays.nullable() }))

export const retentionSchema: ResourceFormSchema<RetentionPayload, RetentionValues> = {
  blank: { retentionDays: '' },
  create: retention,
  update: retention,
  toValues: ({ retentionDays }) => ({ retentionDays: retentionDays ?? '' }),
}
