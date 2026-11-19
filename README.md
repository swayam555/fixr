# Fixr: two-sided home-services marketplace (React Native)

> **Status: work in progress, Phase 2 of 8.** Auth, catalogue, booking and payments are built and tested. Live location, dispatch UI, tracking and the AI features are not finished. Live status is in [`PROGRESS.md`](PROGRESS.md).

Fixr is an Urban Company-style marketplace with two apps, built to production standards:
- **Customer app:** book a plumber, electrician or cleaner, track the pro and pay.
- **Partner app:** go online, accept jobs, navigate and get paid.

## What works today

- **Auth:** OTP login with HMAC-hashed codes, a resend cooldown and per-phone rate limits. RS256 JWTs with JWKS. Rotating refresh tokens with reuse detection that revokes the whole token family.
- **Booking:** a service catalogue and addresses with a map pin. A PostGIS zone check runs over 5 seeded Bengaluru zones. Booking is idempotent and runs an explicit order state machine with optimistic locking and a start code that only the customer sees.
- **Events:** a transactional outbox publishes to Kafka at least once, and every consumer dedupes.
- **Payments (Go):**
  - A Razorpay order per job; concurrent "Pay" taps share one gateway order.
  - HMAC-verified webhooks feed a double-entry ledger. A database trigger rejects any unbalanced journal.
  - Refunds are idempotent and proportional.
- **`razorpay-local`:** an offline Razorpay stand-in that signs webhooks, duplicates every delivery and replays them on demand.
- **Built ahead of schedule** (compiles and unit-tested, not yet wired into the apps): location ingest, H3 location processing, dispatch (Hungarian vs greedy), the realtime gateway and the notifier.

**Tests:** 14 order-service integration tests (Testcontainers), 7 payments tests (including 20 concurrent replays of one webhook leaving the ledger unchanged), 10 core unit tests and 3 component tests.

## Stack

| Layer | Tech |
|---|---|
| Mobile | Expo SDK 58, React Native 0.88 (New Architecture), Expo Router, MapLibre, Reanimated, Skia, MMKV, TanStack Query, Zustand |
| Backend | Spring Boot 4.1 (Java 21), Go services, PostgreSQL + PostGIS, Redis 8, Kafka (KRaft), MinIO, nginx |
| Infra | Docker Compose, GitHub Actions, OpenTelemetry |

## Run it

Prerequisites, every env file and key, and what works without keys are all in **[`SETUP.md`](SETUP.md)**. Short version:

```sh
cp .env.example .env
docker compose up -d --build
scripts/smoke-auth.sh
```

## Docs

[Build plan](docs/PLAN.md) · [Architecture](docs/ARCHITECTURE.md) · [Changelog](docs/CHANGELOG.md) · [Progress](PROGRESS.md)

## License

MIT. See [LICENSE](LICENSE).
