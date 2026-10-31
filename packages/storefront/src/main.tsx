import { MantineProvider } from '@mantine/core'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'

import { CartProvider } from './cart.tsx'
import { Layout } from './Layout.tsx'
import { CartPage } from './pages/CartPage.tsx'
import { CoursesPage } from './pages/CoursesPage.tsx'
import { PricingPage } from './pages/PricingPage.tsx'
import { SignupPage } from './pages/SignupPage.tsx'
import { startTracking, trackNavigations } from './tracking.ts'

import '@mantine/core/styles.css'

startTracking()

const router = createBrowserRouter([
  {
    Component: Layout,
    children: [
      { index: true, Component: CoursesPage },
      { path: 'pricing', Component: PricingPage },
      { path: 'signup', Component: SignupPage },
      { path: 'cart', Component: CartPage },
    ],
  },
])
trackNavigations(router)

const container = document.querySelector('#root')
if (!container) {
  throw new Error('Root element not found')
}

createRoot(container).render(
  <StrictMode>
    <MantineProvider defaultColorScheme="auto">
      <CartProvider>
        <RouterProvider router={router} />
      </CartProvider>
    </MantineProvider>
  </StrictMode>,
)
