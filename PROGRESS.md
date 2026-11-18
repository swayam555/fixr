# Fixr progress

Live status for whoever picks this up next (human or agent). Plan: `docs/PLAN.md`. Design: `docs/ARCHITECTURE.md`.

## Current phase: 1 — Foundation

| Item | Status |
|---|---|
| Monorepo (pnpm 12, Turborepo, hoisted linker) | done |
| packages/ui, packages/core + tests | done (10 core unit tests, 3 RNTL) |
| Customer + Partner apps: Expo Router, OTP login, home | done, typecheck + lint clean |
| order-service auth (OTP, RS256 JWT, JWKS, rotating refresh) | done, 8 Testcontainers ITs |
| Compose: postgres/postgis, redis, kafka + topics, minio, order-service ×2, nginx TLS | done, smoke tested (`scripts/smoke-auth.sh`) |
| CI workflow | written (not run on GitHub; repo is local only) |
| Both apps booting on the Android emulator | NOT verified yet: customer debug APK build was still running at stop (log in session scratchpad); first build failed on my own gradle.properties append, fixed via plugins/with-gradle-tuning.js |

## Phase 3/4 groundwork already written (services/go, compiles + vets, not yet run against the stack)

- location-ingest (protobuf batches, filter.go with jitter/speed/age rules, 5 tests), location-processor (GEO, H3 r8 ZSETs, liveness sweep, fan-out only for active jobs)
- dispatch-service (zone leader lease, expanding GEOSEARCH, ETA client with heuristic fallback, Hungarian vs greedy matcher with brute-force tests, 15 s offers, Lua accept/decline, keyspace-expiry timeouts)
- realtime-gateway (dynamic Redis subscribe per channel, stream resume without loss/reorder, location coalescing, 4008 slow-consumer close, ACL from acl:order:* keys)
- notifier (order.events + dispatch.offers to durable WS frames, order ACLs, FCM v1 or log sender, eventId dedupe)
- h3-go needs cgo and the host has no gcc: use `scripts/go.sh test ./...` (golang container) or install gcc. Packages without h3 run with `CGO_ENABLED=0 go test`.

## Next steps

1. Finish the customer APK build (`FIXR_ANDROID_ABIS=x86_64 FIXR_GRADLE_HEAP=1536m`), install on emulator `bloom_test`, log in against the stack; same for partner. Then commit Phase 1 as done.
2. Phase 2 in order-service: zones/addresses/services, order state machine + outbox poller emitting the envelope the Go services expect (`{eventId,type,orderId,payload{customerId,proId,zoneId,status,version,...}}`), payments-service.
3. Dockerfiles + compose entries for the Go services; Docker Desktop was down (engine socket missing) at stop.

## How to run

```bash
docker compose up -d --build         # first run generates JWT key + local CA into infra/generated/
scripts/smoke-auth.sh                # logs in over HTTP and HTTPS through nginx
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
