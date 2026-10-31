import { Button, Card, List, SimpleGrid, Text, Title } from '@mantine/core'

import { PLANS } from '../courses.ts'

export function PricingPage() {
  return (
    <>
      <Title order={1} mb="md">
        Pricing
      </Title>
      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        {PLANS.map((plan) => (
          <Card key={plan.slug} withBorder>
            <Title order={3}>{plan.title}</Title>
            <Text fw={700} my="xs">
              ${plan.price} / month
            </Text>
            <List mb="md">
              {plan.perks.map((perk) => (
                <List.Item key={perk}>{perk}</List.Item>
              ))}
            </List>
            <Button disabled variant="light">
              Demo plans only
            </Button>
          </Card>
        ))}
      </SimpleGrid>
    </>
  )
}
