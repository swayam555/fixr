CREATE SCHEMA IF NOT EXISTS pay;

-- Filled from order.events (ORDER_COMPLETED): what the customer owes and who earned it.
CREATE TABLE IF NOT EXISTS pay.billables (
    order_id      uuid PRIMARY KEY,
    customer_id   uuid NOT NULL,
    pro_id        uuid NOT NULL,
    amount_paise  bigint NOT NULL CHECK (amount_paise > 0),
    completed_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pay.payments (
    id                  uuid PRIMARY KEY,
    order_id            uuid NOT NULL REFERENCES pay.billables (order_id),
    customer_id         uuid NOT NULL,
    gateway_order_id    text NOT NULL UNIQUE,
    gateway_payment_id  text UNIQUE,
    amount_paise        bigint NOT NULL,
    fee_paise           bigint NOT NULL,
    refunded_paise      bigint NOT NULL DEFAULT 0,
    fee_refunded_paise  bigint NOT NULL DEFAULT 0,
    status              text NOT NULL,
    method              text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (refunded_paise BETWEEN 0 AND amount_paise)
);
-- At most one live attempt per order; failed attempts stay for the audit trail.
CREATE UNIQUE INDEX IF NOT EXISTS payments_one_live ON pay.payments (order_id) WHERE status <> 'FAILED';

CREATE TABLE IF NOT EXISTS pay.refunds (
    id                 uuid PRIMARY KEY,
    payment_id         uuid NOT NULL REFERENCES pay.payments (id),
    gateway_refund_id  text UNIQUE,
    amount_paise       bigint NOT NULL CHECK (amount_paise > 0),
    status             text NOT NULL,
    reason             text,
    idempotency_key    text UNIQUE,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pay.webhook_events (
    event_id     text PRIMARY KEY,
    event_type   text NOT NULL,
    received_at  timestamptz NOT NULL DEFAULT now()
);

-- Double entry: every journal's debits equal its credits (enforced by the trigger below).
CREATE TABLE IF NOT EXISTS pay.journals (
    id          uuid PRIMARY KEY,
    kind        text NOT NULL,
    ref         text NOT NULL,
    order_id    uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, ref)
);

CREATE TABLE IF NOT EXISTS pay.ledger_entries (
    id            bigserial PRIMARY KEY,
    journal_id    uuid NOT NULL REFERENCES pay.journals (id),
    account       text NOT NULL,
    debit_paise   bigint NOT NULL DEFAULT 0 CHECK (debit_paise >= 0),
    credit_paise  bigint NOT NULL DEFAULT 0 CHECK (credit_paise >= 0),
    CHECK ((debit_paise = 0) <> (credit_paise = 0))
);
CREATE INDEX IF NOT EXISTS ledger_entries_account ON pay.ledger_entries (account);
CREATE INDEX IF NOT EXISTS ledger_entries_journal ON pay.ledger_entries (journal_id);

CREATE OR REPLACE FUNCTION pay.check_journal_balanced() RETURNS trigger AS $$
DECLARE diff bigint;
BEGIN
    SELECT coalesce(sum(debit_paise - credit_paise), 0) INTO diff FROM pay.ledger_entries WHERE journal_id = NEW.journal_id;
    IF diff <> 0 THEN
        RAISE EXCEPTION 'journal % is unbalanced by % paise', NEW.journal_id, diff;
    END IF;
    RETURN NULL;
END $$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'ledger_balanced') THEN
        CREATE CONSTRAINT TRIGGER ledger_balanced AFTER INSERT OR UPDATE ON pay.ledger_entries
            DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION pay.check_journal_balanced();
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS pay.outbox (
    id            uuid PRIMARY KEY,
    msg_key       text NOT NULL,
    payload       jsonb NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT clock_timestamp(),
    published_at  timestamptz
);
CREATE INDEX IF NOT EXISTS pay_outbox_pending ON pay.outbox (created_at) WHERE published_at IS NULL;
