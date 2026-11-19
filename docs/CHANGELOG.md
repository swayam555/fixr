# Changelog

## Phase 2 — Catalogue, booking and payments (2026-11-19)

- order-service: service catalogue (5 categories), addresses with map pin plus society/flat, and a PostGIS `ST_Contains` zone check over 5 seeded Bengaluru zones. Booking takes an `Idempotency-Key` and runs an explicit state machine (`SEARCHING → ASSIGNED → EN_ROUTE → ARRIVED → IN_PROGRESS → COMPLETED`, plus `CANCELLED` / `UNFULFILLED`) with optimistic locking, a 4-digit start code only the customer sees, and a transition timeline with the actor on each step.
- Transactional outbox → `order.events`: rows are written in the same transaction as the state change, published under `pg_try_advisory_xact_lock` so one replica publishes at a time, and stamped only after the Kafka ack. Delivery is at-least-once and every consumer dedupes on `eventId`.
- `dispatch.results` listener: duplicate ACCEPTED messages are no-ops; an ACCEPTED that arrives after a cancel emits `ORDER_ASSIGN_REJECTED` so dispatch frees the pro; a pro drop re-dispatches the job and excludes that pro.
- payments-service (Go): Razorpay order per completed job (concurrent "Pay" taps share one gateway order), checkout-signature check, and HMAC-verified webhooks feeding a double-entry ledger (`gateway_clearing`, `platform_fee` at 20 %, `pro_payable:<pro>`). Webhooks are deduped twice: by event id and by `UNIQUE(kind, ref)` on journals. A deferred constraint trigger rejects any journal that does not balance. Refunds are idempotent by key, reserve the amount under `FOR UPDATE`, and split pro share and fee in proportion, with the final refund taking the remainder so the capture reverses exactly.
- razorpay-local: an offline stand-in for the Orders/Refunds API that sends signed webhooks, can duplicate every delivery, and can replay any webhook byte for byte.
- `scripts/smoke-order-pay.sh` drives the whole flow through nginx (not yet run against the full stack at commit time).
- Compose: all seven Go services built from one Dockerfile (distroless, static except the cgo h3 build), with replicas ×2 for ingest, dispatch, realtime-gateway and payments. Every binary supports `-healthcheck`. nginx routes `/api/payments/` and `/pay/webhook`.
- Tests:
  - 6 new order-service ITs, plus the existing 8 auth ITs, all passing. They cover: exactly one `ORDER_CREATED` on Kafka even when the first publish is forced to fail and retried; a full lifecycle with a duplicated dispatch result; a late accept after cancel; drop and redispatch; unserviceable addresses; and other people's orders.
  - 7 payments tests (3 unit, 4 against Postgres). They cover: 20 concurrent replays of one webhook plus the same body under a new event id leave the ledger fingerprint unchanged; refund key reuse and over-refund; concurrent pay taps; the database rejecting an unbalanced journal.
- Fixed before shipping:
  - A refund webhook could beat the gateway-id write, so refunds now also match on our receipt id.
  - A replayed `payment.captured` could rewind a refunded payment, so the capture update now applies only to `CREATED` rows.
  - `payment.failed` closed a gateway order the customer could still retry.

## Phase 1 — Foundation (2026-11-18)

- pnpm 12 + Turborepo monorepo: `apps/customer`, `apps/partner` (Expo SDK 58, RN 0.88, Expo Router, TypeScript strict), `packages/ui` (tokens, theme, primitives, OTP input, shared phone login), `packages/core` (API client with single-flight token refresh, session store on SecureStore, backoff with full jitter).
- order-service (Spring Boot 4.1.1, Java 21): OTP login with HMAC-hashed codes, resend cooldown, per-phone Redis limit and attempt cap; RS256 JWTs with a JWKS endpoint; rotating refresh tokens with reuse detection that revokes the whole family.
- Docker Compose: Postgres 18 + PostGIS 3.7, Redis 8.8, Kafka 4.4 (KRaft, 7 topics created by `kafka-init`), MinIO, two order-service replicas and nginx 1.30 (TLS from a generated local CA, WebSocket upgrade, per-IP rate limits that are stricter on OTP and location routes, DNS-resolved upstreams).
- GitHub Actions CI: typecheck, lint and Jest for the apps, `mvn verify` for order-service; Go and Python jobs switch on once those services exist.
- Tests: 10 core unit tests, 3 RNTL component tests, 8 order-service integration tests (Testcontainers). A bug the tests caught: reusing a revoked refresh token rolled back its own family revocation, because the rejecting exception rolled back the transaction.
