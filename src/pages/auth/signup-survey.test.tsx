import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteSignupSurveySubmit } from '../../generated/site/msw.gen.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route } from '../../routes/signup-survey.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SignupSurveyPage } from './signup-survey.tsx'

function captureSubmissions() {
  const bodies: unknown[] = []
  worker.use(
    handleSiteSignupSurveySubmit(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 204 })
    }),
  )
  return bodies
}

async function renderSurvey() {
  return await renderWithRouter(<SignupSurveyPage />, routeMount(Route))
}

type Screen = Awaited<ReturnType<typeof renderSurvey>>['screen']

// Answers the three required questions (Founder, Just me, Tracking visitors) and lands on the
// optional last screen.
async function answerRequired(screen: Screen) {
  await screen.getByLabelText('Founder').click()
  await screen.getByRole('button', { name: 'Next' }).click()
  await screen.getByLabelText('Just me').click()
  await screen.getByRole('button', { name: 'Next' }).click()
  await screen.getByLabelText('Tracking visitors').click()
  await screen.getByRole('button', { name: 'Next' }).click()
}

test('a required question does not let the User on without an answer', async () => {
  const { screen } = await renderSurvey()

  await expect.element(screen.getByText('What do you do?')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  await screen.getByLabelText('Founder').click()
  await expect.element(screen.getByRole('button', { name: 'Next' })).toBeEnabled()
})

test('the Other choice needs its text before the User can go on', async () => {
  const { screen } = await renderSurvey()

  await screen.getByLabelText('Other').click()
  await expect.element(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  await screen.getByLabelText('Tell us more').fill('   ')
  await expect.element(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  await screen.getByLabelText('Tell us more').fill('Designer')
  await expect.element(screen.getByRole('button', { name: 'Next' })).toBeEnabled()
})

test('no screen has a Skip button', async () => {
  const { screen } = await renderSurvey()

  await expect.element(screen.getByText('What do you do?')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /skip/i }).elements()).toHaveLength(0)
  await answerRequired(screen)
  await expect.element(screen.getByText('Where did you hear about us?')).toBeVisible()
  expect(screen.getByRole('button', { name: /skip/i }).elements()).toHaveLength(0)
})

test('the last question is optional and nothing is sent before it', async () => {
  const bodies = captureSubmissions()
  const { screen } = await renderSurvey()

  await answerRequired(screen)

  await expect.element(screen.getByRole('button', { name: 'Finish' })).toBeEnabled()
  expect(bodies).toEqual([])
})

test('finishing without the optional answer sends the three required ones and leaves', async () => {
  const bodies = captureSubmissions()
  const { screen, navigate } = await renderSurvey()
  await answerRequired(screen)

  await screen.getByRole('button', { name: 'Finish' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ role: 'founder', companySize: '1', mainGoal: 'tracking' }])
  await expect.poll(() => navigate.mock.calls.length).toBeGreaterThan(0)
  expect(navigate).toHaveBeenCalledWith({ to: indexRoute.to })
})

test('sends the optional answer with its trimmed other text', async () => {
  const bodies = captureSubmissions()
  const { screen } = await renderSurvey()
  await answerRequired(screen)

  await screen.getByLabelText('Other').click()
  await screen.getByLabelText('Tell us more').fill('  A podcast ')
  await screen.getByRole('button', { name: 'Finish' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        role: 'founder',
        companySize: '1',
        mainGoal: 'tracking',
        heardFrom: 'other',
        heardFromOther: 'A podcast',
      },
    ])
})

test('a 409 means the survey is not due: the page is left without an error', async () => {
  worker.use(handleSiteSignupSurveySubmit(() => problem(409, 'the sign-up survey is not due')))
  const { screen, navigate } = await renderSurvey()
  await answerRequired(screen)

  await screen.getByRole('button', { name: 'Finish' }).click()

  await expect.poll(() => navigate.mock.calls.length).toBeGreaterThan(0)
  expect(navigate).toHaveBeenCalledWith({ to: indexRoute.to })
  expect(screen.getByText('the sign-up survey is not due').elements()).toHaveLength(0)
})
