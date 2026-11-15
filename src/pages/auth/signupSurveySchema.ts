import * as z from 'zod'

import type {
  SiteSignupCompanySize,
  SiteSignupGoal,
  SiteSignupRole,
  SiteSignupSource,
  SiteSignupSurveyInput,
} from '../../generated/site/types.gen.ts'
import {
  zSiteSignupCompanySize,
  zSiteSignupGoal,
  zSiteSignupRole,
  zSiteSignupSource,
  zSiteSignupSurveyInput,
} from '../../generated/site/zod.gen.ts'
import { trimmed } from '../../resources/schemaForm.ts'

// A question is a field of the input that is not an `…Other` text.
export type Question = Exclude<keyof SiteSignupSurveyInput, `${string}Other`>

const QUESTIONS = ['role', 'companySize', 'mainGoal', 'heardFrom'] as const satisfies Question[]

// The `other` choice every question offers, which asks for a text of its own.
export const OTHER: SiteSignupRole & SiteSignupCompanySize & SiteSignupGoal & SiteSignupSource =
  'other'

// The survey as the form holds it: each question's choice, blank until one is picked, and the
// text of its `other` choice.
export type SurveyValues = Record<Question | `${Question}Other`, string>

export const blankSurvey: SurveyValues = {
  role: '',
  roleOther: '',
  companySize: '',
  companySizeOther: '',
  mainGoal: '',
  mainGoalOther: '',
  heardFrom: '',
  heardFromOther: '',
}

// The text of a question's `other` choice, within the contract's limit.
function otherText(question: Question) {
  return trimmed(zSiteSignupSurveyInput.shape[`${question}Other`].unwrap())
}

// The survey's form schema, the contract's fields read from the form: the first three questions
// need a choice, the last may stay blank, and an `other` choice needs its text. What it hands over
// is the contract's input: an `…Other` text only with its `other` choice, an unanswered optional
// question left out.
export const signupSurveySchema = z
  .object({
    role: z.string().pipe(zSiteSignupRole),
    roleOther: otherText('role'),
    companySize: z.string().pipe(zSiteSignupCompanySize),
    companySizeOther: otherText('companySize'),
    mainGoal: z.string().pipe(zSiteSignupGoal),
    mainGoalOther: otherText('mainGoal'),
    heardFrom: z.string().pipe(z.union([z.literal(''), zSiteSignupSource])),
    heardFromOther: otherText('heardFrom'),
  })
  .superRefine(
    (values, ctx) => {
      for (const question of QUESTIONS) {
        if (values[question] === OTHER && values[`${question}Other`] === '') {
          // The blank text is the issue's input, so it is worded as any missing value is.
          ctx.addIssue({ code: 'custom', path: [`${question}Other`], input: '' })
        }
      }
    },
    // A screen is checked while the later ones are still blank.
    { when: () => true },
  )
  .transform(
    ({
      role,
      roleOther,
      companySize,
      companySizeOther,
      mainGoal,
      mainGoalOther,
      heardFrom,
      heardFromOther,
    }): SiteSignupSurveyInput => ({
      role,
      ...(role === OTHER && { roleOther }),
      companySize,
      ...(companySize === OTHER && { companySizeOther }),
      mainGoal,
      ...(mainGoal === OTHER && { mainGoalOther }),
      ...(heardFrom !== '' && { heardFrom }),
      ...(heardFrom === OTHER && { heardFromOther }),
    }),
  )
