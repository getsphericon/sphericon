import { MantineProvider } from '@mantine/core'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Route, Routes } from 'react-router'

import { CartProvider } from './cart.tsx'
import { Layout } from './Layout.tsx'
import { CartPage } from './pages/CartPage.tsx'
import { CoursesPage } from './pages/CoursesPage.tsx'
import { PricingPage } from './pages/PricingPage.tsx'
import { SignupPage } from './pages/SignupPage.tsx'
import { startTracking } from './tracking.ts'

import '@mantine/core/styles.css'

startTracking()

const container = document.querySelector('#root')
if (!container) {
  throw new Error('Root element not found')
}

createRoot(container).render(
  <StrictMode>
    <MantineProvider defaultColorScheme="auto">
      <CartProvider>
        <BrowserRouter>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<CoursesPage />} />
              <Route path="pricing" element={<PricingPage />} />
              <Route path="signup" element={<SignupPage />} />
              <Route path="cart" element={<CartPage />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </CartProvider>
    </MantineProvider>
  </StrictMode>,
)
