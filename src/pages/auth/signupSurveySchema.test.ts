import { expect, test } from 'vitest'

import { blankSurvey, signupSurveySchema } from './signupSurveySchema.ts'

function issuesOf(values: typeof blankSurvey) {
  const result = signupSurveySchema.safeParse(values)
  return result.success
    ? {}
    : Object.fromEntries(result.error.issues.map((i) => [i.path.join('.'), i.message]))
}

test('an other choice without its text is refused at the text while later questions are blank', () => {
  const issues = issuesOf({ ...blankSurvey, role: 'other', roleOther: '  ' })

  expect(issues).toMatchObject({
    roleOther: 'This field is required',
    companySize: 'This field is required',
  })
})

test('a blank required question is refused, a blank optional one is not', () => {
  const issues = issuesOf(blankSurvey)

  expect(Object.keys(issues).toSorted()).toEqual(['companySize', 'mainGoal', 'role'])
})

test('hands over the other text, trimmed, only with its other choice, and leaves out a blank optional answer', () => {
  const parsed = signupSurveySchema.parse({
    ...blankSurvey,
    role: 'founder',
    roleOther: 'kept from an earlier pick',
    companySize: 'other',
    companySizeOther: '  Nonprofit ',
    mainGoal: 'tracking',
  })

  expect(parsed).toEqual({
    role: 'founder',
    companySize: 'other',
    companySizeOther: 'Nonprofit',
    mainGoal: 'tracking',
  })
})
