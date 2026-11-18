# Changelog

## Phase 1 — Foundation (2026-11-18)

- pnpm 12 + Turborepo monorepo: `apps/customer`, `apps/partner` (Expo SDK 58, RN 0.88, Expo Router, TypeScript strict), `packages/ui` (tokens, theme, primitives, OTP input, shared phone login), `packages/core` (API client with single-flight token refresh, session store on SecureStore, backoff with full jitter).
- order-service (Spring Boot 4.1.1, Java 21): OTP login with HMAC-hashed codes, resend cooldown, per-phone Redis limit and attempt cap; RS256 JWTs with a JWKS endpoint; rotating refresh tokens with reuse detection that revokes the whole family.
- Docker Compose: Postgres 18 + PostGIS 3.7, Redis 8.8, Kafka 4.4 (KRaft, 7 topics created by `kafka-init`), MinIO, two order-service replicas and nginx 1.30 (TLS from a generated local CA, WebSocket upgrade, per-IP rate limits that are stricter on OTP and location routes, DNS-resolved upstreams).
- GitHub Actions CI: typecheck, lint and Jest for the apps, `mvn verify` for order-service; Go and Python jobs switch on once those services exist.
- Tests: 10 core unit tests, 3 RNTL component tests, 8 order-service integration tests (Testcontainers). A bug the tests caught: reusing a revoked refresh token rolled back its own family revocation, because the rejecting exception rolled back the transaction.
