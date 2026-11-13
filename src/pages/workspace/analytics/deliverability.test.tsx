import { expect, test } from 'vitest'

import { handleSiteAnalyticsDeliverability } from '../../../generated/site/msw.gen.ts'
import { Route as deliverabilityRoute } from '../../../routes/workspaces/$workspaceId/analytics/deliverability.tsx'
import { testId } from '../../../test/payloads.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { DeliverabilityPage } from './deliverability.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(deliverabilityRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=7d` }

test('lists each domain with its counts and its rates, an undefined one as a dash', async () => {
  worker.use(
    handleSiteAnalyticsDeliverability({
      body: {
        windowHours: 168,
        domains: [
          {
            sendingDomainId: testId(1),
            domain: 'mail.acme.com',
            complaintRate: { numerator: 3, denominator: 1000, rate: 0.003 },
            bounceRate: { numerator: 20, denominator: 1050, rate: 20 / 1050 },
          },
          {
            sendingDomainId: testId(2),
            domain: 'news.acme.com',
            complaintRate: { numerator: 0, denominator: 0, rate: null },
            bounceRate: { numerator: 1, denominator: 1, rate: 1 },
          },
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<DeliverabilityPage />, mount)

  const mail = screen.getByRole('row', { name: /mail\.acme\.com/ })
  // domain, sent, delivered, hard bounces, complaints, Complaint rate, Bounce rate
  await expect.element(mail).toHaveTextContent('mail.acme.com105010002030.3%1.9%')
  const news = screen.getByRole('row', { name: /news\.acme\.com/ })
  await expect.element(news).toHaveTextContent('news.acme.com1010–100%')
})

test('says so when no domain had traffic in the window', async () => {
  worker.use(handleSiteAnalyticsDeliverability({ body: { windowHours: 168, domains: [] } }))

  const { screen } = await renderWithRouter(<DeliverabilityPage />, mount)

  await expect
    .element(screen.getByText('No Sending domain sent or delivered mail in this window'))
    .toBeInTheDocument()
})
