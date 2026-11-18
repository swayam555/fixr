import { useQuery } from '@tanstack/react-query';
import { Button, Card, Screen, Stack, Text } from '@fixr/ui';
import { auth, useSession } from '@/lib/api';

export default function Home() {
  const user = useSession((s) => s.user);
  const me = useQuery({ queryKey: ['me'], queryFn: auth.me });
  return (
    <Screen>
      <Stack gap="lg" style={{ marginTop: 32 }}>
        <Text variant="title">Signed in</Text>
        <Card>
          <Text testID="me-phone">{me.data?.maskedPhone ?? user?.maskedPhone}</Text>
          <Text tone="muted">{me.data?.role ?? user?.role}</Text>
        </Card>
        <Button kind="secondary" title="Sign out" onPress={() => useSession.getState().signOut()} />
      </Stack>
    </Screen>
  );
}
