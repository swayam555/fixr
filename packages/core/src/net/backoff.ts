export interface BackoffPolicy {
  baseMs: number;
  maxMs: number;
}

export const DEFAULT_BACKOFF: BackoffPolicy = { baseMs: 500, maxMs: 30_000 };

/** "Full jitter": uniform in [0, min(max, base * 2^attempt)]. Spreads reconnect storms after an outage. */
export function backoffDelay(attempt: number, policy = DEFAULT_BACKOFF, random = Math.random): number {
  const ceiling = Math.min(policy.maxMs, policy.baseMs * 2 ** Math.max(0, attempt));
  return Math.round(random() * ceiling);
}
