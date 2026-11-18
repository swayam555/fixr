# Fixr build plan

Each phase ends runnable, tested and committed, with a `docs/CHANGELOG.md` entry. Live status is in `PROGRESS.md` at the repo root.

## Stack (versions checked on npm and Maven Central on 2026-11-18)

| Layer | Choice |
|---|---|
| Mobile | Expo SDK 58, React Native 0.88 (New Architecture, Hermes), React 19.3, TypeScript strict, Expo Router, dev client |
| Mobile libraries | MapLibre RN 11.5, Reanimated 4.7, Skia 2.14, Vision Camera 5, MMKV 4, TanStack Query 5, Zustand 5, @gorhom/bottom-sheet 5, expo-location, expo-task-manager, expo-sqlite, expo-audio, Nitro Modules 0.37, h3-js 4, Sentry RN 8 |
| Monorepo | pnpm 12 workspaces + Turborepo 2 |
| Backend | Spring Boot 4.1.1 on Java 21, Go 1.2x, FastAPI on Python 3.12 (uv), Postgres 17 + PostGIS, Redis 8, Kafka (KRaft), MinIO, Nginx 1.30 |
| Observability | OpenTelemetry, Prometheus, Grafana, Jaeger, Sentry |
| Testing | JUnit 5 + Testcontainers, Go test, pytest, Jest + RNTL, Maestro, k6 |

## Decisions where the brief leaves room

1. **MapLibre, not react-native-maps.** It needs no Google Maps key, it supports the New Architecture in v11, and its data-driven styling makes the heatmap cheap.
2. **Outbox poller, not Debezium.** Debezium Connect needs about 700 MB, which does not fit the 4 GB Docker budget. The poller uses `FOR UPDATE SKIP LOCKED`, so two order-service instances can run it safely. The interview notes compare the two.
3. **Go for the realtime gateway** (the brief allows Go or Node), and for dispatch, payments and the notifier too, so all six small services share one module and one image.
4. **External providers have adapters with local stand-ins.** Each one uses the real provider when its key is set and a clearly labelled local stand-in when it is not:
   - LLM: Claude, then Groq, then a rules engine.
   - Payments: Razorpay test mode, or `razorpay-local`, a stub that speaks the same Orders API and HMAC-signed webhooks.
   - Push: FCM, or delivery through the realtime gateway plus local notifications.
   - STT: a Whisper-compatible endpoint.
   - Errors: Sentry.
   Simulated pros and customers always go through the real APIs and topics.
5. **iOS code is written but not verified.** This machine runs Linux, so the iOS paths (significant-change location, the Swift side of the Nitro module) compile only on EAS. Android is verified on the emulator. The docs say this plainly.
6. **Battery budget method.** An emulator cannot measure battery, so `docs/perf/REPORT.md` gives the exact `dumpsys batterystats` procedure for a real device and records "not yet measured" until one is run. No made-up numbers.

## Phases and acceptance criteria

### 1. Foundation
- pnpm + Turborepo monorepo: `apps/customer`, `apps/partner`, `apps/ops`, `packages/{ui,core,maps,proto}`, `services/{order-service,go,ai-service}`, `simulator`, `infra`.
- Both apps boot on the Android emulator with the shared UI kit (tokens, Button, Text, Card, Sheet) and Expo Router.
- `docker compose up` starts Postgres + PostGIS, Redis, Kafka (KRaft), MinIO, Nginx (TLS with a local CA) and order-service.
- OTP login with JWT access and refresh tokens. The OTP endpoint is rate-limited in Nginx and in the app. In dev the OTP is printed to logs and returned only when `FIXR_DEV_OTP=true`.
- GitHub Actions CI: typecheck, lint, Jest, Maven verify, Go test, pytest.
- **Accept when** a fresh clone can run `docker compose up -d && pnpm dev`, both apps log in on the emulator, and CI passes locally (via `act` or the same scripts).

### 2. Catalogue and booking
- Services catalogue, addresses with a map pin and society/flat details, and a PostGIS `ST_Contains` zone check (Bengaluru zones seeded: Koramangala, HSR, Indiranagar, Whitefield, Jayanagar).
- Order creation with the state machine, optimistic locking and outbox → `order.events`.
- payments-service: Razorpay order creation, signed webhook verification, idempotent double-entry ledger, refunds.
- **Accept when** an integration test creates an order and sees exactly one `ORDER_CREATED` on Kafka, even with a forced publish retry, and a duplicated webhook leaves the ledger unchanged.

### 3. Location pipeline and Partner online mode
- location-ingest (protobuf batches, jitter and speed filters), Kafka `pro.locations`, location-processor (Redis GEO, H3 r8 sets, liveness TTL, fan-out control).
- Partner app: online toggle, background task with an Android foreground service, adaptive frequency state machine, SQLite queue with flush, Nitro ActivityRecognition module.
- **Accept when** the emulator's mock GPS route shows up in Redis GEO within 2 s, and an airplane-mode test flushes every queued point in order after reconnecting.

### 4. Dispatch and live tracking
- dispatch-service: expanding GEOSEARCH, scoring, a 2 s micro-batch with the Hungarian algorithm, 15 s offers with expiry and re-offer, a per-pro lock.
- realtime-gateway ×2: JWT channel auth, Redis pub/sub, stream resume, backpressure.
- Partner app: full-screen offer with a Skia countdown ring and Accept/Decline. Customer app: live map with an interpolated, rotating marker, route polyline, ETA, timeline and bottom sheet.
- **Accept when** a test with 3 orders and 3 pros shows batch matching beating greedy on total ETA, killing one gateway mid-trip loses no state frames, and no pro is ever double-assigned in a 1,000-order soak.

### 5. AI
- Snap-to-Book (photo and voice), Pydantic `JobScope` with retry and fallback, STT, a waveform drawn with Skia.
- Simulator-generated history, LightGBM ETA and duration models (p50/p90), with MAE reported against a straight-line baseline.
- Demand forecast worker and the Partner heatmap. Support copilot with four tools and escalation. Quality check on before and after photos.
- **Accept when** a 30-case scope eval set reaches at least 85% category accuracy (reported honestly, whatever the number), and the ETA model beats the baseline on a held-out set.

### 6. Job lifecycle
- OTP to start, timer, add-on approval, resumable chunked photo uploads to MinIO, completion, rating and tip, PDF invoice, a notification for every state, and an offline action queue with idempotency keys.
- **Accept when** a Partner run in airplane mode through arrive, start and complete replays exactly once on reconnect.

### 7. Scale and hardening
- Simulator at 5,000 pros with a diurnal customer load, k6 for REST and WS, Grafana dashboards, OTel traces, Sentry wiring.
- Maestro E2E driving both apps through booking, tracking and completion against the simulator.
- Failure drills: kill a gateway, kill dispatch mid-offer, cut the Partner's network. Each is written up in `docs/perf/BACKEND.md` and `docs/FAILURE_DRILLS.md`.
- **Accept when** the numbers in `docs/perf/BACKEND.md` and `docs/perf/REPORT.md` are measured on this machine and state their method.

### 8. Publish readiness and the ops web
- Icons, splash screens, store listings, a DPDP privacy policy, the Play background-location justification, EAS profiles (two bundle IDs, update channels) and a demo video script.
- Ops web (Vite): live map of pros and orders, zone editor, stuck-order list.
- README, `docs/INTERVIEW_NOTES.md` and `docs/RESUME_BULLETS.md`.

## Inputs from you (all optional; everything runs without them)

| Key | Unlocks |
|---|---|
| `ANTHROPIC_API_KEY` | Claude for Snap-to-Book, the copilot and quality checks (Groq is used next if its key exists) |
| Razorpay test key ID and secret | real Razorpay test checkout instead of `razorpay-local` |
| Firebase `google-services.json` + service account | real FCM push |
| Sentry DSN | crash and release health |
| Expo account | `eas build` / `eas update` (the config is in place either way) |
