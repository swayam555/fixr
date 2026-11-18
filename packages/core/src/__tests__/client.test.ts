import { ApiError, createApiClient, type Tokens } from '../api/client';
import { normalisePhone } from '../api/auth';
import { backoffDelay } from '../net/backoff';

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function memoryTokens(initial: Tokens | null) {
  let t = initial;
  return { get: () => t, set: (n: Tokens | null) => (t = n) };
}

describe('api client', () => {
  it('refreshes once for concurrent 401s and retries with the new token', async () => {
    const tokens = memoryTokens({ accessToken: 'old', refreshToken: 'r1' });
    let refreshCalls = 0;
    const fetchImpl = jest.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/api/auth/refresh')) {
        refreshCalls++;
        await new Promise((r) => setTimeout(r, 10));
        return jsonResponse(200, { accessToken: 'new', refreshToken: 'r2' });
      }
      const auth = (init.headers as Record<string, string>).Authorization;
      return auth === 'Bearer new' ? jsonResponse(200, { ok: true }) : jsonResponse(401, { code: 'TOKEN_EXPIRED' });
    }) as unknown as typeof fetch;
    const api = createApiClient({ baseUrl: 'http://x', tokens, fetchImpl });

    const results = await Promise.all([api.get('/a'), api.get('/b'), api.get('/c')]);

    expect(results).toEqual([{ ok: true }, { ok: true }, { ok: true }]);
    expect(refreshCalls).toBe(1);
    expect(tokens.get()).toEqual({ accessToken: 'new', refreshToken: 'r2' });
  });

  it('signs out when refresh is rejected', async () => {
    const tokens = memoryTokens({ accessToken: 'old', refreshToken: 'bad' });
    const expired = jest.fn();
    const fetchImpl = (async () => jsonResponse(401, { code: 'INVALID_REFRESH' })) as unknown as typeof fetch;
    const api = createApiClient({ baseUrl: 'http://x', tokens, fetchImpl, onSessionExpired: expired });

    await expect(api.get('/a')).rejects.toMatchObject({ status: 401 });
    expect(tokens.get()).toBeNull();
    expect(expired).toHaveBeenCalledTimes(1);
  });

  it('sends the idempotency key and maps error bodies', async () => {
    const fetchImpl = jest.fn(async () => jsonResponse(409, { code: 'ILLEGAL_TRANSITION', message: 'nope' }));
    const api = createApiClient({ baseUrl: 'http://x', tokens: memoryTokens(null), fetchImpl: fetchImpl as unknown as typeof fetch });

    const err = await api.post<never>('/orders/1/start', {}, { idempotencyKey: 'k-1' }).catch((e: unknown) => e as ApiError);

    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe('ILLEGAL_TRANSITION');
    const init = (fetchImpl.mock.calls[0] as unknown as [string, RequestInit])[1];
    expect((init.headers as Record<string, string>)['Idempotency-Key']).toBe('k-1');
  });

  it('reports network failures with status 0', async () => {
    const fetchImpl = (async () => {
      throw new TypeError('Network request failed');
    }) as unknown as typeof fetch;
    const api = createApiClient({ baseUrl: 'http://x', tokens: memoryTokens(null), fetchImpl });
    const err = await api.get<never>('/a').catch((e: unknown) => e as ApiError);
    expect(err.isNetwork).toBe(true);
  });
});

describe('normalisePhone', () => {
  it.each([
    ['98765 43210', '+919876543210'],
    ['+91 98765-43210', '+919876543210'],
    ['09876543210', '+919876543210'],
    ['12345 67890', null],
    ['98765', null],
  ])('%s -> %s', (input, expected) => expect(normalisePhone(input)).toBe(expected));
});

describe('backoffDelay', () => {
  it('caps at maxMs and never goes negative', () => {
    expect(backoffDelay(0, { baseMs: 500, maxMs: 30_000 }, () => 1)).toBe(500);
    expect(backoffDelay(20, { baseMs: 500, maxMs: 30_000 }, () => 1)).toBe(30_000);
    expect(backoffDelay(3, { baseMs: 500, maxMs: 30_000 }, () => 0)).toBe(0);
  });
});
