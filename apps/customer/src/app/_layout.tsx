import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AppShell } from '@fixr/ui';
import { Stack } from 'expo-router';
import { useSession } from '@/lib/api';

const queryClient = new QueryClient({ defaultOptions: { queries: { staleTime: 30_000, retry: 2 } } });

export default function RootLayout() {
  const signedIn = useSession((s) => !!s.tokens);
  const hydrated = useSession((s) => s.hydrated);
  return (
    <AppShell ready={hydrated}>
      <QueryClientProvider client={queryClient}>
        <Stack screenOptions={{ headerShown: false }}>
          <Stack.Protected guard={signedIn}>
            <Stack.Screen name="(app)" />
          </Stack.Protected>
          <Stack.Protected guard={!signedIn}>
            <Stack.Screen name="(auth)" />
          </Stack.Protected>
        </Stack>
      </QueryClientProvider>
    </AppShell>
  );
}
