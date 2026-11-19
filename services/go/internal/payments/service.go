package payments

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//go:embed schema.sql
var schema string

const TopicPayments = "payments.events"

var (
	ledgerJournals = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fixr_ledger_journals_total", Help: "Journals posted to the ledger, by kind."}, []string{"kind"})
	webhooksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fixr_payment_webhooks_total", Help: "Gateway webhooks by event and outcome."}, []string{"event", "outcome"})
)

var (
	ErrNotBillable   = errors.New("order is not billable yet")
	ErrAlreadyPaid   = errors.New("order already paid")
	ErrNotPaid       = errors.New("order has no captured payment")
	ErrRefundTooBig  = errors.New("refund exceeds what is left on the payment")
	ErrBadSignature  = errors.New("signature mismatch")
	ErrPaymentAbsent = errors.New("payment not found")
)

type Service struct {
	DB            *pgxpool.Pool
	Gateway       Gateway
	KeySecret     string
	WebhookSecret string
	FeeBps        int
}

// Migrate applies schema.sql under an advisory lock so replicas starting together do not race.
func (s *Service) Migrate(ctx context.Context) error {
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext('fixr.payments.schema'))"); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext('fixr.payments.schema'))")
	_, err = conn.Exec(ctx, schema)
	return err
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.DB, fn)
}

// OrderEvent is the subset of order-service's envelope payments needs.
type OrderEvent struct {
	EventID string `json:"eventId"`
	Type    string `json:"type"`
	OrderID string `json:"orderId"`
	Payload struct {
		CustomerID string `json:"customerId"`
		ProID      string `json:"proId"`
		PricePaise int64  `json:"pricePaise"`
	} `json:"payload"`
}

// OnOrderEvent opens a bill when a job completes. Upsert-by-order makes redelivery harmless.
func (s *Service) OnOrderEvent(ctx context.Context, ev OrderEvent) error {
	if ev.Type != "ORDER_COMPLETED" {
		return nil
	}
	if ev.Payload.PricePaise <= 0 || ev.Payload.ProID == "" {
		slog.Warn("completed order without price or pro", "order", ev.OrderID)
		return nil
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO pay.billables (order_id, customer_id, pro_id, amount_paise)
		VALUES ($1, $2, $3, $4) ON CONFLICT (order_id) DO NOTHING`,
		ev.OrderID, ev.Payload.CustomerID, ev.Payload.ProID, ev.Payload.PricePaise)
	return err
}

type Payment struct {
	ID               string    `json:"id"`
	OrderID          string    `json:"orderId"`
	GatewayOrderID   string    `json:"gatewayOrderId"`
	GatewayPaymentID *string   `json:"gatewayPaymentId,omitempty"`
	AmountPaise      int64     `json:"amountPaise"`
	RefundedPaise    int64     `json:"refundedPaise"`
	Status           string    `json:"status"`
	Method           *string   `json:"method,omitempty"`
	KeyID            string    `json:"keyId,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

const paymentCols = `id, order_id, gateway_order_id, gateway_payment_id, amount_paise, refunded_paise, status, method, updated_at`

func scanPayment(row pgx.Row) (*Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.OrderID, &p.GatewayOrderID, &p.GatewayPaymentID, &p.AmountPaise, &p.RefundedPaise, &p.Status, &p.Method, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPaymentAbsent
	}
	return &p, err
}

func (s *Service) livePayment(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, orderID string, lock bool) (*Payment, error) {
	sql := `SELECT ` + paymentCols + ` FROM pay.payments WHERE order_id = $1 AND status <> 'FAILED'`
	if lock {
		sql += " FOR UPDATE"
	}
	return scanPayment(q.QueryRow(ctx, sql, orderID))
}

// CreateIntent returns the gateway order the app opens checkout with. Calling it again returns the
// same gateway order until it is paid or fails, so a double-tap on "Pay" never charges twice.
func (s *Service) CreateIntent(ctx context.Context, orderID, customerID string) (*Payment, error) {
	var amount int64
	var owner string
	err := s.DB.QueryRow(ctx, `SELECT amount_paise, customer_id::text FROM pay.billables WHERE order_id = $1`, orderID).Scan(&amount, &owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != customerID) {
		return nil, ErrNotBillable
	}
	if err != nil {
		return nil, err
	}
	if p, err := s.livePayment(ctx, s.DB, orderID, false); err == nil {
		return s.existing(p)
	} else if !errors.Is(err, ErrPaymentAbsent) {
		return nil, err
	}

	gw, err := s.Gateway.CreateOrder(ctx, amount, orderID, map[string]string{"fixr_order_id": orderID})
	if err != nil {
		return nil, fmt.Errorf("gateway create order: %w", err)
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO pay.payments (id, order_id, customer_id, gateway_order_id, amount_paise, fee_paise, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'CREATED')`, newUUID(), orderID, customerID, gw.ID, amount, FeeFor(amount, s.FeeBps))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// a concurrent request won; its gateway order is the one to use (ours simply expires unpaid)
		p, err := s.livePayment(ctx, s.DB, orderID, false)
		if err != nil {
			return nil, err
		}
		return s.existing(p)
	}
	if err != nil {
		return nil, err
	}
	p, err := s.livePayment(ctx, s.DB, orderID, false)
	if err != nil {
		return nil, err
	}
	p.KeyID = s.Gateway.KeyID()
	return p, nil
}

func (s *Service) existing(p *Payment) (*Payment, error) {
	if p.Status != "CREATED" {
		return p, ErrAlreadyPaid
	}
	p.KeyID = s.Gateway.KeyID()
	return p, nil
}

func (s *Service) Status(ctx context.Context, orderID, customerID string) (*Payment, error) {
	return scanPayment(s.DB.QueryRow(ctx, `SELECT `+paymentCols+` FROM pay.payments
		WHERE order_id = $1 AND customer_id = $2 ORDER BY created_at DESC LIMIT 1`, orderID, customerID))
}

// ConfirmCheckout only checks the checkout signature so the app can show "confirming". Money moves
// in the ledger when the signed webhook arrives, never on the client's word.
func (s *Service) ConfirmCheckout(ctx context.Context, customerID, gatewayOrderID, gatewayPaymentID, signature string) (*Payment, error) {
	if !VerifyCheckout(s.KeySecret, gatewayOrderID, gatewayPaymentID, signature) {
		return nil, ErrBadSignature
	}
	return scanPayment(s.DB.QueryRow(ctx, `SELECT `+paymentCols+` FROM pay.payments
		WHERE gateway_order_id = $1 AND customer_id = $2`, gatewayOrderID, customerID))
}

type webhook struct {
	Event   string `json:"event"`
	Payload struct {
		Payment *struct {
			Entity struct {
				ID      string `json:"id"`
				OrderID string `json:"order_id"`
				Amount  int64  `json:"amount"`
				Method  string `json:"method"`
			} `json:"entity"`
		} `json:"payment"`
		Refund *struct {
			Entity struct {
				ID        string `json:"id"`
				PaymentID string `json:"payment_id"`
				Amount    int64  `json:"amount"`
				Receipt   string `json:"receipt"`
			} `json:"entity"`
		} `json:"refund"`
	} `json:"payload"`
}

// HandleWebhook applies one verified gateway event exactly once. The event id insert, the state
// change, the journal and the outbox row share a transaction: a crash anywhere rolls all of it back
// and the gateway's retry starts clean; a duplicate finds the event id taken and changes nothing.
func (s *Service) HandleWebhook(ctx context.Context, eventID string, body []byte) (applied bool, err error) {
	var w webhook
	if err := json.Unmarshal(body, &w); err != nil {
		return false, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO pay.webhook_events (event_id, event_type) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, eventID, w.Event)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		applied = true
		switch w.Event {
		case "payment.captured":
			return s.onCaptured(ctx, tx, w)
		case "payment.failed":
			// the gateway order stays open and the customer can retry it, so nothing to change
			return nil
		case "refund.processed":
			return s.onRefundProcessed(ctx, tx, w)
		case "refund.failed":
			if w.Payload.Refund != nil {
				re := w.Payload.Refund.Entity
				_, err = tx.Exec(ctx, `UPDATE pay.refunds SET status = 'FAILED', gateway_refund_id = $1, updated_at = now()
					WHERE (gateway_refund_id = $1 OR id::text = $2) AND status = 'PENDING'`, re.ID, re.Receipt)
			}
			return err
		}
		return nil
	})
	outcome := "applied"
	switch {
	case err != nil:
		outcome = "error"
	case !applied:
		outcome = "duplicate"
	}
	webhooksTotal.WithLabelValues(w.Event, outcome).Inc()
	return applied, err
}

func (s *Service) onCaptured(ctx context.Context, tx pgx.Tx, w webhook) error {
	if w.Payload.Payment == nil {
		return nil
	}
	pe := w.Payload.Payment.Entity
	var id, orderID, proID string
	var amount, fee int64
	err := tx.QueryRow(ctx, `SELECT p.id, p.order_id, b.pro_id::text, p.amount_paise, p.fee_paise
		FROM pay.payments p JOIN pay.billables b USING (order_id) WHERE p.gateway_order_id = $1 FOR UPDATE OF p`, pe.OrderID).
		Scan(&id, &orderID, &proID, &amount, &fee)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("capture for unknown gateway order", "gateway_order", pe.OrderID)
		return nil
	}
	if err != nil {
		return err
	}
	if pe.Amount != amount {
		return fmt.Errorf("captured %d paise but expected %d for %s", pe.Amount, amount, pe.OrderID)
	}
	tag, err := tx.Exec(ctx, `UPDATE pay.payments SET status = 'CAPTURED', gateway_payment_id = $2, method = $3, updated_at = now()
		WHERE id = $1 AND status = 'CREATED'`, id, pe.ID, pe.Method)
	if err != nil || tag.RowsAffected() == 0 {
		return err // already captured (and maybe refunded since): a replay must not rewind it
	}
	posted, err := postJournal(ctx, tx, "capture", pe.ID, orderID, CaptureEntries(amount, fee, proID))
	if err != nil || !posted {
		return err
	}
	return s.emit(ctx, tx, orderID, "PAYMENT_CAPTURED", map[string]any{"paymentId": id, "amountPaise": amount, "method": pe.Method})
}

func (s *Service) onRefundProcessed(ctx context.Context, tx pgx.Tx, w webhook) error {
	if w.Payload.Refund == nil {
		return nil
	}
	re := w.Payload.Refund.Entity
	var payID, orderID, proID string
	var amount, fee, refunded, feeRefunded int64
	err := tx.QueryRow(ctx, `SELECT p.id, p.order_id, b.pro_id::text, p.amount_paise, p.fee_paise, p.refunded_paise, p.fee_refunded_paise
		FROM pay.payments p JOIN pay.billables b USING (order_id) WHERE p.gateway_payment_id = $1 FOR UPDATE OF p`, re.PaymentID).
		Scan(&payID, &orderID, &proID, &amount, &fee, &refunded, &feeRefunded)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("refund for unknown payment", "gateway_payment", re.PaymentID)
		return nil
	}
	if err != nil {
		return err
	}
	if refunded+re.Amount > amount {
		return fmt.Errorf("refund %s would take %s past its captured amount", re.ID, re.PaymentID)
	}
	// a refund raised from the gateway dashboard has no row yet; record it so the audit trail is whole
	// match by gateway id, or by our own id sent as the receipt (the webhook can beat our own update)
	tag, err := tx.Exec(ctx, `UPDATE pay.refunds SET status = 'PROCESSED', gateway_refund_id = $1, updated_at = now()
		WHERE (gateway_refund_id = $1 OR id::text = $2) AND status <> 'PROCESSED'`, re.ID, re.Receipt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO pay.refunds (id, payment_id, gateway_refund_id, amount_paise, status, reason)
			VALUES ($1, $2, $3, $4, 'PROCESSED', 'gateway dashboard') ON CONFLICT (gateway_refund_id) DO NOTHING`,
			newUUID(), payID, re.ID, re.Amount); err != nil {
			return err
		}
	}
	feePart := RefundSplit(amount, fee, refunded, feeRefunded, re.Amount)
	posted, err := postJournal(ctx, tx, "refund", re.ID, orderID, RefundEntries(re.Amount, feePart, proID))
	if err != nil || !posted {
		return err
	}
	status := "PARTIALLY_REFUNDED"
	if refunded+re.Amount == amount {
		status = "REFUNDED"
	}
	if _, err := tx.Exec(ctx, `UPDATE pay.payments SET refunded_paise = refunded_paise + $2, fee_refunded_paise = fee_refunded_paise + $3,
		status = $4, updated_at = now() WHERE id = $1`, payID, re.Amount, feePart, status); err != nil {
		return err
	}
	return s.emit(ctx, tx, orderID, "REFUND_PROCESSED", map[string]any{"paymentId": payID, "refundId": re.ID, "amountPaise": re.Amount})
}

type Refund struct {
	ID              string  `json:"id"`
	GatewayRefundID *string `json:"gatewayRefundId,omitempty"`
	AmountPaise     int64   `json:"amountPaise"`
	Status          string  `json:"status"`
}

// RequestRefund reserves the amount against the payment (so two ops agents cannot over-refund),
// then asks the gateway. The ledger moves later, on refund.processed.
func (s *Service) RequestRefund(ctx context.Context, orderID, idemKey string, amount int64, reason string) (*Refund, error) {
	var r Refund
	var gatewayPaymentID string
	fresh := false
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id, gateway_refund_id, amount_paise, status FROM pay.refunds WHERE idempotency_key = $1`, idemKey).
			Scan(&r.ID, &r.GatewayRefundID, &r.AmountPaise, &r.Status)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		p, err := s.livePayment(ctx, tx, orderID, true)
		if errors.Is(err, ErrPaymentAbsent) {
			return ErrNotPaid
		}
		if err != nil {
			return err
		}
		if p.GatewayPaymentID == nil || (p.Status != "CAPTURED" && p.Status != "PARTIALLY_REFUNDED") {
			return ErrNotPaid
		}
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT coalesce(sum(amount_paise), 0) FROM pay.refunds WHERE payment_id = $1 AND status = 'PENDING'`, p.ID).
			Scan(&reserved); err != nil {
			return err
		}
		if amount <= 0 || p.RefundedPaise+reserved+amount > p.AmountPaise {
			return ErrRefundTooBig
		}
		r = Refund{ID: newUUID(), AmountPaise: amount, Status: "PENDING"}
		gatewayPaymentID = *p.GatewayPaymentID
		fresh = true
		_, err = tx.Exec(ctx, `INSERT INTO pay.refunds (id, payment_id, amount_paise, status, reason, idempotency_key)
			VALUES ($1, $2, $3, 'PENDING', $4, $5)`, r.ID, p.ID, amount, reason, idemKey)
		return err
	})
	if err != nil || !fresh {
		return &r, err
	}
	gr, err := s.Gateway.Refund(ctx, gatewayPaymentID, amount, r.ID)
	if err != nil {
		_, _ = s.DB.Exec(ctx, `UPDATE pay.refunds SET status = 'FAILED', updated_at = now() WHERE id = $1 AND status = 'PENDING'`, r.ID)
		r.Status = "FAILED"
		return &r, fmt.Errorf("gateway refund: %w", err)
	}
	// the webhook may already have landed and moved the status on; only fill in the gateway id
	_, err = s.DB.Exec(ctx, `UPDATE pay.refunds SET gateway_refund_id = $2, updated_at = now() WHERE id = $1`, r.ID, gr.ID)
	r.GatewayRefundID = &gr.ID
	return &r, err
}

type Balance struct {
	Account string `json:"account"`
	Paise   int64  `json:"paise"`
}

// TrialBalance lists every account's debit-positive balance; the total is always zero.
func (s *Service) TrialBalance(ctx context.Context, prefix string) ([]Balance, int64, error) {
	rows, err := s.DB.Query(ctx, `SELECT account, sum(debit_paise - credit_paise)::bigint FROM pay.ledger_entries
		WHERE account LIKE $1 || '%' GROUP BY account ORDER BY account`, prefix)
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Balance, error) {
		var b Balance
		return b, r.Scan(&b.Account, &b.Paise)
	})
	var total int64
	for _, b := range out {
		total += b.Paise
	}
	return out, total, err
}

func (s *Service) emit(ctx context.Context, tx pgx.Tx, orderID, typ string, payload map[string]any) error {
	js, _ := json.Marshal(map[string]any{"eventId": newUUID(), "type": typ, "orderId": orderID,
		"atMs": time.Now().UnixMilli(), "payload": payload})
	_, err := tx.Exec(ctx, `INSERT INTO pay.outbox (id, msg_key, payload) VALUES ($1, $2, $3)`, newUUID(), orderID, js)
	return err
}
