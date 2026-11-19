CREATE TABLE zones (
    id      varchar(32) PRIMARY KEY,
    name    varchar(64) NOT NULL,
    city    varchar(32) NOT NULL,
    geom    geometry(Polygon, 4326) NOT NULL,
    active  boolean NOT NULL DEFAULT true
);
CREATE INDEX zones_geom ON zones USING gist (geom);

-- Rough neighbourhood boxes for Bengaluru; ops can redraw them later without a deploy.
INSERT INTO zones (id, name, city, geom) VALUES
 ('blr-koramangala', 'Koramangala', 'Bengaluru', ST_MakeEnvelope(77.605, 12.920, 77.640, 12.945, 4326)),
 ('blr-hsr',         'HSR Layout',  'Bengaluru', ST_MakeEnvelope(77.625, 12.895, 77.660, 12.920, 4326)),
 ('blr-indiranagar', 'Indiranagar', 'Bengaluru', ST_MakeEnvelope(77.630, 12.960, 77.655, 12.985, 4326)),
 ('blr-whitefield',  'Whitefield',  'Bengaluru', ST_MakeEnvelope(77.720, 12.955, 77.770, 12.995, 4326)),
 ('blr-jayanagar',   'Jayanagar',   'Bengaluru', ST_MakeEnvelope(77.570, 12.915, 77.600, 12.945, 4326));

CREATE TABLE service_categories (
    id      varchar(32) PRIMARY KEY,
    name    varchar(48) NOT NULL,
    icon    varchar(32) NOT NULL,
    sort    int NOT NULL
);

CREATE TABLE services (
    id              varchar(48) PRIMARY KEY,
    category_id     varchar(32) NOT NULL REFERENCES service_categories (id),
    name            varchar(80) NOT NULL,
    description     varchar(280) NOT NULL,
    skill           varchar(32) NOT NULL,
    skill_level     int NOT NULL CHECK (skill_level BETWEEN 1 AND 3),
    price_paise     int NOT NULL CHECK (price_paise > 0),
    est_minutes     int NOT NULL,
    tools           text[] NOT NULL DEFAULT '{}',
    active          boolean NOT NULL DEFAULT true,
    sort            int NOT NULL
);

INSERT INTO service_categories (id, name, icon, sort) VALUES
 ('plumbing', 'Plumbing', 'droplet', 1),
 ('electrical', 'Electrical', 'zap', 2),
 ('cleaning', 'Cleaning', 'sparkles', 3),
 ('appliance', 'Appliances', 'washing-machine', 4),
 ('carpentry', 'Carpentry', 'hammer', 5);

INSERT INTO services (id, category_id, name, description, skill, skill_level, price_paise, est_minutes, tools, sort) VALUES
 ('tap-leak',        'plumbing',   'Leaking tap fix',          'Washer or cartridge swap on one tap. Parts billed at MRP.', 'plumbing', 1, 19900, 30, '{wrench,ptfe-tape}', 1),
 ('drain-block',     'plumbing',   'Blocked drain',            'Sink, basin or floor trap cleared with a plunger or snake.', 'plumbing', 2, 34900, 45, '{drain-snake,plunger}', 2),
 ('flush-repair',    'plumbing',   'Flush tank repair',        'Running or weak flush, fill valve or flapper replaced.', 'plumbing', 2, 29900, 40, '{wrench}', 3),
 ('switch-socket',   'electrical', 'Switch or socket swap',    'Replace up to two switches or sockets on the same board.', 'electrical', 1, 14900, 25, '{tester,screwdriver}', 1),
 ('fan-install',     'electrical', 'Ceiling fan install',      'Mount and wire one ceiling fan. Hook must already exist.', 'electrical', 2, 29900, 45, '{drill,tester,ladder}', 2),
 ('mcb-trip',        'electrical', 'MCB keeps tripping',       'Find the faulty circuit and fix or isolate it.', 'electrical', 3, 39900, 60, '{multimeter,tester}', 3),
 ('bathroom-clean',  'cleaning',   'Bathroom deep clean',      'One bathroom: tiles, fittings, mirror and floor descaled.', 'cleaning', 1, 49900, 75, '{scrubber,descaler}', 1),
 ('kitchen-clean',   'cleaning',   'Kitchen deep clean',       'Slab, sink, chimney exterior and cabinet fronts degreased.', 'cleaning', 2, 79900, 120, '{degreaser,scrubber}', 2),
 ('ac-service',      'appliance',  'AC service',               'Split or window AC: filter, coil and drain-line wash.', 'appliance', 2, 59900, 60, '{jet-pump,gauge}', 1),
 ('washer-checkup',  'appliance',  'Washing machine check-up', 'Diagnose noise, leaks or drain faults. Repair quoted on site.', 'appliance', 2, 29900, 40, '{multimeter,screwdriver}', 2),
 ('door-hinge',      'carpentry',  'Door hinge or lock fix',   'Sagging door, loose hinge or stuck lock on one door.', 'carpentry', 1, 24900, 35, '{drill,screwdriver}', 1),
 ('furniture-assembly','carpentry','Furniture assembly',       'Flat-pack bed, wardrobe or table, up to one item.', 'carpentry', 2, 59900, 90, '{drill,allen-keys}', 2);

CREATE TABLE addresses (
    id          uuid PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    label       varchar(24) NOT NULL,
    line1       varchar(160) NOT NULL,
    society     varchar(80),
    flat        varchar(32),
    landmark    varchar(80),
    lat         double precision NOT NULL,
    lng         double precision NOT NULL,
    location    geography(Point, 4326) GENERATED ALWAYS AS (ST_SetSRID(ST_MakePoint(lng, lat), 4326)::geography) STORED,
    zone_id     varchar(32) REFERENCES zones (id),
    deleted_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX addresses_user ON addresses (user_id) WHERE deleted_at IS NULL;

CREATE TABLE orders (
    id                uuid PRIMARY KEY,
    customer_id       uuid NOT NULL REFERENCES users (id),
    pro_id            uuid REFERENCES users (id),
    service_id        varchar(48) NOT NULL REFERENCES services (id),
    zone_id           varchar(32) NOT NULL REFERENCES zones (id),
    address_id        uuid NOT NULL REFERENCES addresses (id),
    address_summary   varchar(200) NOT NULL,
    lat               double precision NOT NULL,
    lng               double precision NOT NULL,
    status            varchar(16) NOT NULL,
    version           bigint NOT NULL DEFAULT 0,
    price_paise       int NOT NULL,
    notes             varchar(500),
    scope             jsonb,
    start_code        varchar(4) NOT NULL,
    eta_sec           int,
    redispatches      int NOT NULL DEFAULT 0,
    cancel_reason     varchar(200),
    idempotency_key   varchar(64) NOT NULL,
    searching_at      timestamptz,
    assigned_at       timestamptz,
    arrived_at        timestamptz,
    started_at        timestamptz,
    completed_at      timestamptz,
    closed_at         timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (customer_id, idempotency_key)
);
CREATE INDEX orders_customer ON orders (customer_id, created_at DESC);
CREATE INDEX orders_pro ON orders (pro_id, created_at DESC) WHERE pro_id IS NOT NULL;
CREATE INDEX orders_open ON orders (status) WHERE status NOT IN ('COMPLETED', 'CANCELLED', 'UNFULFILLED');

CREATE TABLE order_transitions (
    id          uuid PRIMARY KEY,
    order_id    uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status varchar(16),
    to_status   varchar(16) NOT NULL,
    actor       varchar(16) NOT NULL,
    actor_id    uuid,
    note        varchar(200),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_transitions_order ON order_transitions (order_id, created_at);

-- Written in the same transaction as the state change; a poller publishes and stamps published_at.
CREATE TABLE outbox (
    id            uuid PRIMARY KEY,
    topic         varchar(48) NOT NULL,
    msg_key       varchar(64) NOT NULL,
    event_type    varchar(32) NOT NULL,
    payload       jsonb NOT NULL,
    attempts      int NOT NULL DEFAULT 0,
    last_error    varchar(300),
    created_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    published_at  timestamptz
);
CREATE INDEX outbox_pending ON outbox (created_at) WHERE published_at IS NULL;

CREATE TABLE processed_events (
    consumer    varchar(32) NOT NULL,
    event_id    varchar(64) NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
