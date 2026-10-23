import { createContext, use, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import type { Course } from './courses.ts'

interface Cart {
  items: readonly Course[]
  add: (course: Course) => void
  remove: (slug: string) => void
}

const CartContext = createContext<Cart | null>(null)

export function CartProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<readonly Course[]>([])
  const value = useMemo<Cart>(
    () => ({
      items,
      add: (course) => {
        setItems((current) =>
          current.some((item) => item.slug === course.slug) ? current : [...current, course],
        )
      },
      remove: (slug) => {
        setItems((current) => current.filter((item) => item.slug !== slug))
      },
    }),
    [items],
  )

  return <CartContext value={value}>{children}</CartContext>
}

export function useCart(): Cart {
  const cart = use(CartContext)
  if (!cart) {
    throw new Error('useCart needs a CartProvider')
  }

  return cart
}
