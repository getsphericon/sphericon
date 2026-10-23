import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import { CartContext } from './cartContext.ts'
import type { Cart } from './cartContext.ts'
import type { Course } from './courses.ts'

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
