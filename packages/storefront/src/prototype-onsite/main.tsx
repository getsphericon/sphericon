// PROTOTYPE (throwaway, branch prototype/onsite-form-builder, ticket #958).
// Run: pnpm --filter @sphericon/storefront prototype  →  http://localhost:5199/prototype/onsite-builder
// The shop pages are the real storefront pages, without the tracker (no dev stack needed).
import { MantineProvider } from '@mantine/core'
import { StrictMode, useEffect, useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, useLocation } from 'react-router'
import { RouterProvider } from 'react-router/dom'

import { CartProvider } from '../cart.tsx'
import { Layout } from '../Layout.tsx'
import { CartPage } from '../pages/CartPage.tsx'
import { CoursesPage } from '../pages/CoursesPage.tsx'
import { PricingPage } from '../pages/PricingPage.tsx'
import { Builder } from './Builder.tsx'
import { startOnsite } from './runtime.ts'
import { ArticlePage, DebugPanel } from './ShopPages.tsx'

import '@mantine/core/styles.css'

function Shop() {
  const location = useLocation()
  const [log, setLog] = useState<string[]>([])
  const [forceMobile, setForceMobile] = useState(() => sessionStorage.getItem('PROTOTYPE_mobile') === '1')
  const mobile = useRef(forceMobile)
  mobile.current = forceMobile
  useEffect(() => {
    sessionStorage.setItem('PROTOTYPE_mobile', forceMobile ? '1' : '0')
  }, [forceMobile])
  useEffect(
    () =>
      startOnsite(
        () => (mobile.current || matchMedia('(max-width: 640px)').matches ? 'mobile' : 'desktop'),
        (line) => setLog((l) => [`${new Date().toLocaleTimeString()} ${line}`, ...l].slice(0, 40)),
        () => false,
      ),
    // A new page view per navigation (ADR 0092: one overlay per page view).
    [location.pathname, forceMobile],
  )

  return (
    <>
      <Layout />
      <DebugPanel log={log} forceMobile={forceMobile} setForceMobile={setForceMobile} />
    </>
  )
}

const router = createBrowserRouter([
  { path: '/prototype/onsite-builder', Component: Builder },
  {
    Component: Shop,
    children: [
      { index: true, Component: CoursesPage },
      { path: 'pricing', Component: PricingPage },
      { path: 'cart', Component: CartPage },
      { path: 'prototype/article', Component: ArticlePage },
    ],
  },
])

createRoot(document.querySelector('#root') as HTMLElement).render(
  <StrictMode>
    <MantineProvider defaultColorScheme="light">
      <CartProvider>
        <RouterProvider router={router} />
      </CartProvider>
    </MantineProvider>
  </StrictMode>,
)
