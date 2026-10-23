import { createContext, use } from 'react'

import type { Course } from './courses.ts'

export interface Cart {
  items: readonly Course[]
  add: (course: Course) => void
  remove: (slug: string) => void
}

export const CartContext = createContext<Cart | null>(null)

export function useCart(): Cart {
  const cart = use(CartContext)
  if (!cart) {
    throw new Error('useCart needs a CartProvider')
  }

  return cart
}
