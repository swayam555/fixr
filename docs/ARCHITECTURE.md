# Fixr architecture

Fixr is a two-sided marketplace that gets a verified pro to a customer's door in 10 to 15 minutes. It has two React Native apps (Customer and Partner), one event-driven backend and a city simulator that drives that backend through its public APIs.

## 1. System

```mermaid
flowchart LR
  subgraph Clients
    C[Customer app<br/>Expo / RN 0.88]
    P[Partner app<br/>Expo / RN 0.88]
    O[Ops web<br/>Vite + React]
    SIM[City simulator<br/>Go, 5k pros]
  end

  C & P & O & SIM -->|HTTPS / WSS| NGX[Nginx<br/>TLS, WS upgrade,<br/>rate limits]

  NGX -->|/api| ORD[order-service ×2<br/>Spring Boot 4, Java 21]
  NGX -->|/ws| GW[realtime-gateway ×2<br/>Go]
  NGX -->|/loc| ING[location-ingest<br/>Go, protobuf]
  NGX -->|/ai| AI[ai-service<br/>FastAPI]
  NGX -->|/pay/webhook| PAY[payments-service<br/>Go]

  ORD --> PG[(Postgres 17<br/>+ PostGIS)]
  ORD -->|outbox poller| K{{Kafka KRaft}}
  ING -->|pro.locations<br/>key = pro_id| K
  K --> LP[location-processor<br/>Go]
  LP -->|GEOADD, H3 sets,<br/>liveness TTL| R[(Redis 8)]
  LP -->|pro:id only while on a job| R
  K -->|order.events| DSP[dispatch-service<br/>Go]
  DSP -->|GEOSEARCH 1→3→5 km| R
  DSP -->|ETA scoring| AI
  DSP -->|dispatch.offers| K
  K -->|dispatch.results| ORD
  K --> NOT[notifier<br/>Go → FCM]
  K --> PAY
  PAY --> PG
  AI -->|forecast per hex, 5 min| R
  R -->|pub/sub| GW
  ORD & AI -->|presigned URLs| S3[(MinIO)]
  K -->|ml.features| AI
```

| Service | Language | Owns | Why that language |
|---|---|---|---|
| order-service | Java 21, Spring Boot 4 | users, OTP and JWT auth, catalogue, addresses, zones, orders, outbox, ratings, media metadata, invoices, support tickets | rich domain model, JPA optimistic locking, transactions around outbox writes |
| location-ingest | Go | nothing (stateless) | hot path: thousands of points per second, small memory |
| location-processor | Go | Redis GEO + H3 membership | same reason |
| dispatch-service | Go | dispatch_offers, matching loop | tight 2 s loop, Hungarian solver, Redis locks |
| realtime-gateway | Go | WebSocket sessions | about 10 KB per connection, goroutine per socket |
| payments-service | Go | payments, ledger_entries | small, webhook-driven |
| notifier | Go | nothing (stateless) | Kafka to FCM fan-out |
| ai-service | Python, FastAPI | ML models, LLM calls, forecast worker | LightGBM, h3, LLM SDKs |

All Go services share one module (`services/go`) with one binary per `cmd/`, so the protobuf types, Kafka client and auth code are shared.

## 2. Order state machine

```mermaid
stateDiagram-v2
  [*] --> CREATED: POST /orders (payment intent created)
  CREATED --> SEARCHING: PAYMENT_AUTHORIZED / pay-later
  CREATED --> CANCELLED: customer cancels / payment failed
  SEARCHING --> ASSIGNED: dispatch.results ACCEPTED
  SEARCHING --> UNFULFILLED: no pro within 5 km after N rounds
  SEARCHING --> CANCELLED: customer cancels
  ASSIGNED --> EN_ROUTE: pro taps "Start trip"
  ASSIGNED --> SEARCHING: pro cancels (re-dispatch)
  EN_ROUTE --> ARRIVED: arrival geofence (50 m) or manual
  EN_ROUTE --> SEARCHING: pro cancels (re-dispatch)
  ARRIVED --> IN_PROGRESS: customer OTP verified
  IN_PROGRESS --> COMPLETED: after-photos + complete
  COMPLETED --> [*]
  CANCELLED --> [*]
  UNFULFILLED --> [*]
```

- Transitions live in one table (`OrderStateMachine.ALLOWED`). Any other transition returns `409 ILLEGAL_TRANSITION`.
- `orders.version` is a JPA `@Version` column, so a transition that races another fails with `409 STALE_ORDER` and the client refetches.
- Each transition writes `orders`, `order_events` and `outbox` in one transaction. A poller (`SELECT ... FOR UPDATE SKIP LOCKED`, 200 ms) publishes `outbox` rows to `order.events`, keyed by `order_id`, and marks them sent. It delivers at least once. Consumers dedupe on `processed_events(consumer, event_id)`.

## 3. Dispatch sequence

```mermaid
sequenceDiagram
  autonumber
  participant C as Customer app
  participant OS as order-service
  participant K as Kafka
  participant D as dispatch-service
  participant R as Redis
  participant AI as ai-service (ETA)
  participant G as realtime-gateway
  participant P as Partner app

  C->>OS: POST /orders (AI scope, address)
  OS->>OS: tx: order + order_event + outbox
  OS-->>K: order.events ORDER_SEARCHING (poller)
  K->>D: consume (idempotent)
  D->>D: add to zone's open-order pool
  loop every 2 s per zone
    D->>R: GEOSEARCH 1 km, then 3, then 5 (idle, live, skill)
    D->>AI: batch ETA predict (pros × orders)
    D->>D: score = ETA + rating + utilisation + skill match<br/>Hungarian assignment across the pool
    D->>R: SET lock:pro:{id} NX PX 16000
    D->>R: SET offer:{id} PX 15000
    D->>K: dispatch.offers OFFERED
  end
  K->>G: offer → pro:{id} channel (via notifier + Redis pub/sub)
  G->>P: full-screen offer, 15 s ring
  alt pro accepts within 15 s
    P->>D: POST /dispatch/offers/{id}/accept (idempotency key)
    D->>K: dispatch.results ACCEPTED
    K->>OS: ASSIGNED (optimistic lock)
    OS-->>K: order.events ORDER_ASSIGNED
    K->>G: order:{id} → customer timeline + map
  else declines or offer key expires
    R-->>D: keyspace expiry / decline
    D->>R: release lock, exclude pro for this order
    D->>D: back in the pool for the next round
  end
```

## 4. Location pipeline

```mermaid
flowchart LR
  A[Partner app<br/>bg task] -->|batch, protobuf,<br/>≤ 50 pts| ING[location-ingest]
  A -.->|offline| Q[(SQLite queue)] -.->|flush on reconnect| ING
  ING -->|drop: speed > 120 km/h,<br/>accuracy > 100 m, out of order| X[discard + metric]
  ING -->|Kafka key = pro_id| K[pro.locations]
  K --> LP[location-processor]
  LP --> G1[GEOADD pros:geo]
  LP --> G2[SADD hex:r8:id pro<br/>pro:hex, pro:seen TTL 60 s]
  LP -->|only if pro has active order| PUB[PUBLISH pro:id]
  LP --> F[ml.features]
```

- Points are partitioned by `pro_id`, so each pro's points are processed in order.
- `pro:seen:{id}` has a 60 s TTL. A pro whose key expired is treated as offline by dispatch, whatever the app says.
- Fan-out control: a location update goes to the `pro:{id}` pub/sub channel only while that pro has an active job (`active:pro:{id}` set by the order consumer). Idle pros update only the GEO/H3 indices, which keeps gateway traffic proportional to live jobs rather than online pros.

## 5. Realtime gateway

- One WebSocket per app. The client sends `{"op":"sub","ch":"order:123"}` and the gateway checks the channel against JWT claims: customers get `order:{own}`, pros get `pro:{self}` and `offer:{self}`, everyone gets `hex:*`, ops gets everything.
- Every frame carries `seq`, a per-channel sequence from a Redis Stream (`XADD ch:{name} MAXLEN ~ 200`). On reconnect the client sends `{"op":"resume","ch":...,"after":seq}` and the gateway replays from the stream, so no node needs sticky sessions.
- Backpressure: each socket has a bounded send queue. Location frames are coalesced per channel (only the latest is kept). State frames are never dropped. A socket that falls behind 256 frames is closed with code 4008 and the client reconnects with resume.
- Client reconnect: exponential backoff (0.5 s to 30 s) with full jitter, paused while the app is in the background and resumed on `AppState` active.

## 6. Mobile architecture

```mermaid
flowchart TB
  subgraph monorepo
    UI[packages/ui<br/>tokens, primitives, Skia ring, timeline]
    CORE[packages/core<br/>API client, WS client, query hooks,<br/>Zustand stores, MMKV, offline queue]
    MAPS[packages/maps<br/>MapLibre, AnimatedProMarker,<br/>H3 heatmap, snap-to-route]
    NAT[modules/activity-recognition<br/>Nitro module: Kotlin + Swift]
    CA[apps/customer] --> UI & CORE & MAPS
    PA[apps/partner] --> UI & CORE & MAPS & NAT
  end
```

- **Maps**: MapLibre RN v11 with OpenFreeMap vector tiles, so no Google Maps key is needed. The pro marker is a `MarkerView` whose position is a Reanimated shared value. Each GPS fix starts a `withTiming` from the current point to the new one over the gap between fixes, and the bearing rotates along the shortest arc. A fix within 30 m of the route polyline is projected onto it.
- **Heatmap**: `h3-js` turns the visible bbox into r8 cells (`polygonToCells`). One `ShapeSource` holds the polygons and the fill colour is a data-driven expression on `feature-state`, so a WebSocket update changes one feature's state without rebuilding the GeoJSON.
- **Background location**: an `expo-task-manager` task with an Android foreground service. A state machine (`EN_ROUTE` 3 s, `ON_JOB` 15 s, `IDLE` 30 s, `STATIONARY` paused) is driven by job state plus the Nitro `ActivityRecognition` module (still, walking or vehicle). Points go into an `expo-sqlite` queue and are flushed in batches of up to 50.
- **Offline actions**: arrive, start, complete and photo upload are queued with a client UUID as `Idempotency-Key` and replayed strictly in order. The server stores `(key, response)` for 24 h, so a replay returns the original response.
- **State**: TanStack Query for server state (with optimistic updates on add-ons and ratings), Zustand for session and UI, MMKV as the persister for both.

## 7. Data model

Postgres tables: `users`, `otp_challenges`, `pros`, `pro_skills`, `pro_documents`, `zones` (PostGIS `geometry(Polygon,4326)`), `addresses`, `services`, `orders`, `order_events`, `outbox`, `processed_events`, `idempotency_keys`, `dispatch_offers`, `payments`, `ledger_entries`, `ratings`, `media`, `support_tickets`, `consents`.

Redis keys: `pros:geo` (GEO), `hex:r8:{cell}` (set), `pro:hex:{id}`, `pro:seen:{id}` (TTL 60 s), `pro:state:{id}` (hash: online, skills, rating, jobs today), `lock:pro:{id}`, `offer:{id}`, `active:pro:{id}`, `forecast:{cell}`, streams `ch:{channel}`.

Kafka topics (6 partitions each, keyed as noted): `order.events` (order_id), `pro.locations` (pro_id), `dispatch.offers` (pro_id), `dispatch.results` (order_id), `payments.events` (order_id), `notifications.out` (user_id), `ml.features` (order_id).

## 8. AI service

| Endpoint | What it does | Model |
|---|---|---|
| `POST /ai/scope` | photo, video frame or text in; a `JobScope` JSON out | multimodal LLM behind a `LLMProvider` interface (Claude by default, Groq second, deterministic rules if neither key is set) |
| `POST /ai/stt` | audio in, Hinglish transcript out | Whisper-compatible endpoint |
| `POST /ai/eta` | batch of (pro, order) features in; ETA and duration p50/p90 out | LightGBM quantile models trained on simulator history |
| `POST /ai/support` | tool-calling copilot | LLM with tools `get_order`, `get_pro_location`, `reschedule`, `raise_ticket` |
| `POST /ai/quality` | before and after photos in; verdict and confidence out | vision LLM |
| worker | demand forecast per r8 hex for the next 60 min, every 5 min | seasonal baseline + LightGBM on recent counts |

`JobScope` is validated with Pydantic. An invalid reply is retried once with the validation error fed back. If confidence is below 0.6, the app falls back to the manual category picker.

## 9. Observability

OpenTelemetry SDKs in every service export to an OTel Collector, which sends traces to Jaeger and metrics to Prometheus. Grafana shows orders/min, time-to-assign p50/p95, Kafka consumer lag, active pros, WebSocket connections and location points/sec. Mobile apps report to Sentry (DSN optional) and send custom metrics (location batch success rate, WS reconnects, time to first assignment) to `POST /api/telemetry`.

## 10. Local resource budget

The dev machine gives Docker 4 GB, so the compose file caps memory: Kafka 640 MB (heap 384 MB), Postgres 256 MB, Redis 192 MB, MinIO 192 MB, order-service ×2 at 448 MB each, ai-service 384 MB, Go services 64 MB each, and Prometheus, Grafana, Jaeger and the collector at 128 MB each. Observability is a compose profile (`--profile obs`) so the core stack fits on its own.
