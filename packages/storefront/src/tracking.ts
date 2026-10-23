import { identify, initTracking, track, trackPageView } from '@sphericon/analytics'

// The tracker posts to the app origin (SPHERICON_APP_URL), not to the `api.` host: Caddy rewrites
// every path there to /api/*, so /collect would resolve to /api/collect.
export function startTracking(): void {
  initTracking({
    collectKey: import.meta.env.DEMO_COLLECT_KEY,
    baseUrl: import.meta.env.DEMO_COLLECT_URL,
  })
}

export function pageViewed(): void {
  trackPageView({
    path: location.pathname,
    url: location.href,
    title: document.title,
    referrer: document.referrer,
    locale: navigator.language,
  })
}

export function viewedPricing(plan: string): void {
  track('viewed_pricing', { plan })
}

export function addedToCart(course: string, price: number): void {
  track('add_to_cart', { course, price })
}

// The subject id has the shape of the Brightpath fixtures (`user:<email>`). The storefront's
// server will send `purchase` with the same email and subject id, so both sides land on one
// Contact (ADR 0002).
export function subjectIdFor(email: string): string {
  return `user:${email}`
}

export async function signedUp(email: string): Promise<void> {
  await identify({ email, subjectId: subjectIdFor(email) })
  track('signup')
}
