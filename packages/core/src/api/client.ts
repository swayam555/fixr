export interface Tokens {
  accessToken: string;
  refreshToken: string;
}

export interface TokenStore {
  get(): Tokens | null;
  set(tokens: Tokens | null): void;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly body?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  get isNetwork() {
    return this.status === 0;
  }
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  idempotencyKey?: string;
  auth?: boolean;
  signal?: AbortSignal;
  headers?: Record<string, string>;
}

export interface ApiClientConfig {
  baseUrl: string;
  tokens: TokenStore;
  fetchImpl?: typeof fetch;
  onSessionExpired?: () => void;
  timeoutMs?: number;
}

export type ApiClient = ReturnType<typeof createApiClient>;

export function createApiClient(config: ApiClientConfig) {
  const doFetch = config.fetchImpl ?? fetch;
  const timeoutMs = config.timeoutMs ?? 15_000;
  let refreshing: Promise<Tokens | null> | null = null;

  async function raw(path: string, opts: RequestOptions, accessToken?: string): Promise<Response> {
    const headers: Record<string, string> = { Accept: 'application/json', ...opts.headers };
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
    if (opts.idempotencyKey) headers['Idempotency-Key'] = opts.idempotencyKey;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    opts.signal?.addEventListener('abort', () => controller.abort());
    try {
      return await doFetch(config.baseUrl + path, {
        method: opts.method ?? (opts.body === undefined ? 'GET' : 'POST'),
        headers,
        body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
        signal: controller.signal,
      });
    } catch (e) {
      throw new ApiError(0, 'NETWORK', e instanceof Error ? e.message : 'Network error');
    } finally {
      clearTimeout(timer);
    }
  }

  async function refresh(): Promise<Tokens | null> {
    const current = config.tokens.get();
    if (!current) return null;
    const res = await raw('/api/auth/refresh', { method: 'POST', body: { refreshToken: current.refreshToken } });
    if (!res.ok) {
      config.tokens.set(null);
      config.onSessionExpired?.();
      return null;
    }
    const next = (await res.json()) as Tokens;
    config.tokens.set({ accessToken: next.accessToken, refreshToken: next.refreshToken });
    return next;
  }

  function refreshOnce() {
    refreshing ??= refresh().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  async function parse<T>(res: Response): Promise<T> {
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    const body = text ? JSON.parse(text) : undefined;
    if (!res.ok) {
      const err = (body ?? {}) as { code?: string; message?: string };
      throw new ApiError(res.status, err.code ?? `HTTP_${res.status}`, err.message ?? res.statusText, body);
    }
    return body as T;
  }

  async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    const useAuth = opts.auth !== false;
    let res = await raw(path, opts, useAuth ? config.tokens.get()?.accessToken : undefined);
    if (res.status === 401 && useAuth && config.tokens.get()) {
      const next = await refreshOnce();
      if (next) res = await raw(path, opts, next.accessToken);
    }
    return parse<T>(res);
  }

  return {
    request,
    get: <T>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) => request<T>(path, { ...opts, method: 'GET' }),
    post: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
      request<T>(path, { ...opts, method: 'POST', body: body ?? {} }),
    put: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
      request<T>(path, { ...opts, method: 'PUT', body: body ?? {} }),
    del: <T>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) => request<T>(path, { ...opts, method: 'DELETE' }),
    baseUrl: config.baseUrl,
  };
}
