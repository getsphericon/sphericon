import { Button, Card, SimpleGrid, Text, Title } from '@mantine/core'

import { useCart } from '../cart.tsx'
import { COURSES } from '../courses.ts'
import { addedToCart } from '../tracking.ts'

export function CoursesPage() {
  const cart = useCart()

  return (
    <>
      <Title order={1} mb="md">
        Courses
      </Title>
      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        {COURSES.map((course) => {
          const inCart = cart.items.some((item) => item.slug === course.slug)

          return (
            <Card key={course.slug} withBorder>
              <Title order={3}>{course.title}</Title>
              <Text c="dimmed" my="xs">
                {course.blurb}
              </Text>
              <Text fw={700} mb="md">
                ${course.price}
              </Text>
              <Button
                disabled={inCart}
                onClick={() => {
                  cart.add(course)
                  addedToCart(course.title, course.price)
                }}
              >
                {inCart ? 'In your cart' : 'Add to cart'}
              </Button>
            </Card>
          )
        })}
      </SimpleGrid>
    </>
  )
}
