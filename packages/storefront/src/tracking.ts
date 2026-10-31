import { identify, initTracking, track, trackPageView } from '@sphericon/analytics'
import type { DataRouter, Location } from 'react-router'

import { ENROLL_PATH } from '../server/enroll.ts'
import type { EnrollRequest } from '../server/enroll.ts'

// The tracker posts to the `collect.` host (COLLECT_HOST), not to the `api.` host: Caddy rewrites
// every path there to /api/*, so /collect would resolve to /api/collect.
export function startTracking(): void {
  initTracking({
    collectKey: import.meta.env.DEMO_COLLECT_KEY,
    baseUrl: import.meta.env.DEMO_COLLECT_URL,
  })
}

// A page view on load and on every navigation, and the pricing view on the pricing page. The
// router reports every change of its state; a navigation is the one that brings a new location.
export function trackNavigations(router: DataRouter): void {
  let viewed: Location | null = null
  const view = (current: Location) => {
    if (current === viewed) {
      return
    }
    viewed = current
    trackPageView({
      path: location.pathname,
      url: location.href,
      title: document.title,
      referrer: document.referrer,
      locale: navigator.language,
    })
    if (current.pathname === '/pricing') {
      track('viewed_pricing', { plan: 'pro' })
    }
  }
  view(router.state.location)
  router.subscribe((state) => view(state.location))
}

export function addedToCart(course: string, price: number): void {
  track('add_to_cart', { course, price })
}

// The subject id has the shape of the Brightpath fixtures (`user:<email>`). The storefront's
// server sends `purchase` with the same email and subject id, so both sides land on one
// Contact (ADR 0002).
export function subjectIdFor(email: string): string {
  return `user:${email}`
}

export async function signUp(email: string): Promise<void> {
  await identify({ email, subjectId: subjectIdFor(email) })
  track('signup')
}

// "Enroll" identifies the person in the browser, then asks the storefront's server to record the
// purchase: the server sends the same email and subject id, so front and back events share a Contact.
// A visitor who enrols without signing up first has no identity yet; `identify` gives the browser's
// earlier page views and cart events one.
export async function enroll(email: string, courses: readonly string[]): Promise<void> {
  const subjectId = subjectIdFor(email)
  await identify({ email, subjectId })
  const body: EnrollRequest = { email, subjectId, courses: [...courses] }
  const response = await fetch(ENROLL_PATH, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!response.ok) {
    throw new Error(`Enroll failed: ${response.status}`)
  }
}
