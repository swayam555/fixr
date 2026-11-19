# Fixr progress

Live status for whoever picks this up next (human or agent). Plan: `docs/PLAN.md`. Design: `docs/ARCHITECTURE.md`.

## Current phase: 3 — Location pipeline (Phase 2 done 2026-11-19)

| Phase | Status |
|---|---|
| 1 Foundation | done, except booting both APKs on emulator `bloom_test` (see next steps) |
| 2 Catalogue, booking, payments | done: 14 order-service ITs, 7 payments tests. `scripts/smoke-order-pay.sh` (end to end through nginx) is written but NOT yet run: host memory was exhausted by another session |

Phase 3/4 groundwork already in services/go (compiles, vets, unit tests pass, now in compose): location-ingest, location-processor (h3, cgo), dispatch-service (Hungarian vs greedy, 15 s offers, leader lease), realtime-gateway (stream resume, backpressure), notifier.

## Next steps

1. Run `docker compose up -d --build` then `scripts/smoke-order-pay.sh` (needs ~2.5 GB free); fix anything it finds.
2. Phase 1 leftover: customer and partner APKs (`FIXR_ANDROID_ABIS=x86_64 FIXR_GRADLE_HEAP=1536m`, stack stopped), install on `bloom_test`, log in against the stack.
3. Phase 3 per docs/PLAN.md: Partner online mode (foreground service, adaptive frequency, SQLite queue, Nitro ActivityRecognition) + location pipeline acceptance (mock GPS in Redis GEO < 2 s, airplane-mode flush in order).
4. Booking UI in the customer app (catalogue, address pin, book, pay sheet) can land with Phase 4's tracking screens.

## Payments notes

- Ledger moves only on verified webhooks; `/api/payments/confirm` just checks the checkout signature for fast UI.
- Local gateway: `razorpay-local` on :8097 (`POST /_local/checkout {order_id}` pays, `GET /_local/webhooks`, `POST /_local/webhooks/{id}/replay`). It duplicates every webhook by default (`DUPLICATE_WEBHOOKS`).
- Payments ITs: `PAYMENTS_TEST_DATABASE_URL=postgres://fixr:fixr@localhost:5432/fixr CGO_ENABLED=0 go test ./internal/payments` (creates and drops a throwaway database per test).

## How to run

```bash
docker compose up -d --build         # first run generates JWT key + local CA into infra/generated/
scripts/smoke-auth.sh                # logs in over HTTP and HTTPS through nginx
scripts/smoke-order-pay.sh           # book -> job -> pay -> replayed webhook -> refund
pnpm install && pnpm dev             # Metro for both apps (dev client builds: `pnpm --filter @fixr/customer android`)
```

## Machine gotchas

- Docker Desktop has 4 GB and the host has 7 GB, shared with other sessions (Arth, Dukaan, Kaarkhana). Stop the stack (`docker compose stop`) before Gradle builds. One `expo install` was OOM-killed (exit 137) while another session was running Gradle.
- pnpm 12 ignores `.npmrc`, so `nodeLinker: hoisted` and `allowBuilds` live in `pnpm-workspace.yaml`.
- RN Testing Library 14: `render` and `fireEvent` are async, so await them.
- ESLint 10 + eslint-plugin-react: set `settings.react.version` or React version detection crashes.
- nginx: a `proxy_set_header` inside a location drops every inherited one. All shared headers live in `infra/nginx/fixr-locations.conf.inc` at http level.
- MinIO's own Docker Hub image is gone, so we use `cgr.dev/chainguard/minio:latest` (free tier only offers `latest`).
- Android build settings for this machine (x86_64 only, Gradle heap 1.5 GB) come from `plugins/with-gradle-tuning.js`: set `FIXR_ANDROID_ABIS=x86_64 FIXR_GRADLE_HEAP=1536m` before `expo prebuild`.
