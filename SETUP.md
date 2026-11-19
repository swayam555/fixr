# Setup: requirements and keys

## Prerequisites

| Tool | Version | For |
|---|---|---|
| Node + pnpm | Node ≥ 22, pnpm 12 | monorepo, both Expo apps |
| Java | 21 (Maven wrapper included) | `services/order-service` |
| Go | 1.2x | `services/go` (payments, dispatch, location, realtime, notifier) |
| Docker | ~2.5 GB free RAM for the full stack | Postgres/PostGIS, Redis, Kafka, MinIO, nginx and all services |
| Android Studio / emulator | API 34+ | running the customer and partner apps |

## Where each key goes

| File | Copy from | Read by |
|---|---|---|
| `.env` (repo root) | `.env.example` | `docker compose` |
| `apps/customer/.env` | `apps/customer/.env.example` | Expo, customer app |
| `apps/partner/.env` | `apps/partner/.env.example` | Expo, partner app |

**No paid key is needed to run anything that exists today.** Every external provider has a local stand-in:

| Provider | Variables | Without a key |
|---|---|---|
| Payments (Razorpay test mode) | `RAZORPAY_BASE`, `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, `RAZORPAY_WEBHOOK_SECRET` | `razorpay-local` on :8097 speaks the same Orders API and sends HMAC-signed webhooks |
| SMS OTP | `FIXR_DEV_OTP=true`, `OTP_HMAC_SECRET` | The OTP comes back as `devOtp` in the `/api/auth/otp` response (dev only) |
| Maps | none | MapLibre with open tiles, so no Google Maps key |
| Push (FCM) | `FCM_CREDENTIALS_FILE` (path to a Firebase service-account JSON) on the `notifier` service | Pushes are logged by the notifier; in-app events still arrive through the realtime gateway. Not yet passed through `docker-compose.yml`; add it to the `notifier` environment and mount the file when you need it |
| JWT signing | none | An RS256 key pair is generated into `infra/generated/` (git-ignored) on first run |
| EAS builds | `EAS_PROJECT_ID_CUSTOMER`, `EAS_PROJECT_ID_PARTNER` | Local dev-client builds work without them |

**AI keys:** none yet. The AI features in `docs/PLAN.md` (Claude → Groq → rules-engine fallback) belong to later phases that aren't built. When they land, `ANTHROPIC_API_KEY` and `GROQ_API_KEY` will go in the root `.env`, and the rules engine will run without them.

## Run

```sh
cp .env.example .env
cp apps/customer/.env.example apps/customer/.env && cp apps/partner/.env.example apps/partner/.env
docker compose up -d --build        # nginx on :8080 (HTTP) and :8443 (TLS)
scripts/smoke-auth.sh               # OTP login over HTTP and HTTPS
scripts/smoke-order-pay.sh          # book → job → pay → replayed webhook → refund
pnpm install && pnpm dev            # Metro for both apps
pnpm --filter @fixr/customer android
```

On this 7 GB laptop, Android builds need `FIXR_ANDROID_ABIS=x86_64 FIXR_GRADLE_HEAP=1536m` with the Docker stack stopped (see `PROGRESS.md`).
