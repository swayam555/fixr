package payments_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"fixr.in/svc/internal/payments"
	"fixr.in/svc/internal/rzplocal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs against a real Postgres: PAYMENTS_TEST_DATABASE_URL=postgres://fixr:fixr@localhost:5432/fixr
// Each run creates and drops its own database.
func testDB(t *testing.T) *pgxpool.Pool {
	admin := os.Getenv("PAYMENTS_TEST_DATABASE_URL")
	if admin == "" {
		t.Skip("PAYMENTS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("pay_it_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	cfg, _ := pgxpool.ParseConfig(admin)
	cfg.ConnConfig.Database = name
	cfg.MaxConns = 12
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = conn.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		conn.Close(ctx)
	})
	return pool
}

type rig struct {
	svc *payments.Service
	gw  *httptest.Server
	rzp *rzplocal.Server
}

func newRig(t *testing.T) *rig {
	db := testDB(t)
	rzp := rzplocal.New("rzp_test_k", "ksec", "whsec", "")
	rzp.Delay = 0
	gw := httptest.NewServer(rzp.Routes())
	svc := &payments.Service{DB: db, Gateway: payments.NewRazorpay(gw.URL, "rzp_test_k", "ksec"),
		KeySecret: "ksec", WebhookSecret: "whsec", FeeBps: 2000}
	if err := svc.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Migrate(context.Background()); err != nil {
		t.Fatalf("schema is not re-runnable: %v", err)
	}
	hook := httptest.NewServer(http.HandlerFunc(svc.WebhookHandler))
	rzp.WebhookURL = hook.URL
	t.Cleanup(func() { gw.Close(); hook.Close() })
	return &rig{svc: svc, gw: gw, rzp: rzp}
}

func uuid(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func (r *rig) completed(t *testing.T, order, customer, pro string, paise int64) {
	ev := payments.OrderEvent{EventID: "e", Type: "ORDER_COMPLETED", OrderID: order}
	ev.Payload.CustomerID, ev.Payload.ProID, ev.Payload.PricePaise = customer, pro, paise
	for range 2 {
		if err := r.svc.OnOrderEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
}

func (r *rig) post(t *testing.T, path string, body any) map[string]any {
	js, _ := json.Marshal(body)
	res, err := http.Post(r.gw.URL+path, "application/json", bytes.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out
}

func (r *rig) waitStatus(t *testing.T, order, customer, want string) *payments.Payment {
	deadline := time.Now().Add(10 * time.Second)
	for {
		p, err := r.svc.Status(context.Background(), order, customer)
		if err == nil && p.Status == want {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("payment never reached %s (last %+v, %v)", want, p, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ledger returns a fingerprint of every entry so "unchanged" means byte-for-byte unchanged.
func (r *rig) ledger(t *testing.T) string {
	var s string
	err := r.svc.DB.QueryRow(context.Background(), `SELECT count(*) || '|' || coalesce(string_agg(account || '=' || debit_paise || '/' || credit_paise, ',' ORDER BY id), '')
		FROM pay.ledger_entries`).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDuplicateWebhooksLeaveLedgerUnchanged(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	order, customer, pro := uuid(1), uuid(2), uuid(3)
	r.completed(t, order, customer, pro, 19900)

	if _, err := r.svc.CreateIntent(ctx, order, uuid(99)); !errors.Is(err, payments.ErrNotBillable) {
		t.Fatalf("someone else's order: got %v", err)
	}
	p1, err := r.svc.CreateIntent(ctx, order, customer)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := r.svc.CreateIntent(ctx, order, customer)
	if p1.GatewayOrderID != p2.GatewayOrderID || p1.AmountPaise != 19900 {
		t.Fatalf("second Pay tap made a new gateway order: %s vs %s", p1.GatewayOrderID, p2.GatewayOrderID)
	}

	co := r.post(t, "/_local/checkout", map[string]any{"order_id": p1.GatewayOrderID})
	if _, err := r.svc.ConfirmCheckout(ctx, customer, p1.GatewayOrderID, co["razorpay_payment_id"].(string), "bad"); !errors.Is(err, payments.ErrBadSignature) {
		t.Fatalf("forged checkout signature: got %v", err)
	}
	if _, err := r.svc.ConfirmCheckout(ctx, customer, p1.GatewayOrderID, co["razorpay_payment_id"].(string), co["razorpay_signature"].(string)); err != nil {
		t.Fatal(err)
	}
	r.waitStatus(t, order, customer, "CAPTURED")
	before := r.ledger(t)
	if before[:2] != "3|" {
		t.Fatalf("capture should post 3 entries, ledger: %s", before)
	}

	var sent []struct {
		EventID string          `json:"eventId"`
		Body    json.RawMessage `json:"body"`
	}
	res, _ := http.Get(r.gw.URL + "/_local/webhooks")
	_ = json.NewDecoder(res.Body).Decode(&sent)
	res.Body.Close()
	captured := sent[len(sent)-1]

	// 1) the gateway redelivers the same event (same id, same signature), 20 times at once
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.post(t, "/_local/webhooks/"+captured.EventID+"/replay", nil)
		}()
	}
	wg.Wait()
	// 2) the same capture arrives under a new event id: the (kind, ref) journal key still holds
	if applied, err := r.svc.HandleWebhook(ctx, "evt_new_id", captured.Body); err != nil || !applied {
		t.Fatalf("new event id should be accepted as an event: %v %v", applied, err)
	}
	if after := r.ledger(t); after != before {
		t.Fatalf("ledger changed on duplicate delivery\nbefore %s\nafter  %s", before, after)
	}

	// 3) a forged webhook is refused before it touches the database
	forged := httptest.NewRequest(http.MethodPost, "/pay/webhook", bytes.NewReader(captured.Body))
	forged.Header.Set("X-Razorpay-Signature", payments.SignWebhook("guess", captured.Body))
	rec := httptest.NewRecorder()
	r.svc.WebhookHandler(rec, forged)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged webhook got %d", rec.Code)
	}

	bs, total, _ := r.svc.TrialBalance(ctx, "")
	want := map[string]int64{payments.AccGatewayClearing: 19900, payments.AccPlatformRevenue: -3980, payments.AccProPayable(pro): -15920}
	for _, b := range bs {
		if want[b.Account] != b.Paise {
			t.Fatalf("%s = %d, want %d", b.Account, b.Paise, want[b.Account])
		}
	}
	if total != 0 || len(bs) != 3 {
		t.Fatalf("trial balance total %d over %d accounts", total, len(bs))
	}
	if _, err := r.svc.CreateIntent(ctx, order, customer); !errors.Is(err, payments.ErrAlreadyPaid) {
		t.Fatalf("paying a paid order: got %v", err)
	}
}

func TestRefundsAreIdempotentAndReverseTheLedger(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	order, customer, pro := uuid(11), uuid(12), uuid(13)
	r.completed(t, order, customer, pro, 19900)
	p, err := r.svc.CreateIntent(ctx, order, customer)
	if err != nil {
		t.Fatal(err)
	}
	r.post(t, "/_local/checkout", map[string]any{"order_id": p.GatewayOrderID})
	r.waitStatus(t, order, customer, "CAPTURED")

	a, err := r.svc.RequestRefund(ctx, order, "ops-key-1", 5000, "late arrival")
	if err != nil {
		t.Fatal(err)
	}
	again, err := r.svc.RequestRefund(ctx, order, "ops-key-1", 5000, "late arrival")
	if err != nil || again.ID != a.ID {
		t.Fatalf("retried refund made a new one: %v %v", again, err)
	}
	if _, err := r.svc.RequestRefund(ctx, order, "ops-key-2", 15000, "too much"); !errors.Is(err, payments.ErrRefundTooBig) {
		t.Fatalf("over-refund: got %v", err)
	}
	r.waitStatus(t, order, customer, "PARTIALLY_REFUNDED")
	if _, err := r.svc.RequestRefund(ctx, order, "ops-key-3", 14900, "goodwill"); err != nil {
		t.Fatal(err)
	}
	r.waitStatus(t, order, customer, "REFUNDED")

	bs, total, _ := r.svc.TrialBalance(ctx, "")
	for _, b := range bs {
		if b.Paise != 0 {
			t.Fatalf("%s left at %d after a full refund", b.Account, b.Paise)
		}
	}
	var journals, outbox, refunds int
	_ = r.svc.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM pay.journals), (SELECT count(*) FROM pay.outbox),
		(SELECT count(*) FROM pay.refunds WHERE status = 'PROCESSED')`).Scan(&journals, &outbox, &refunds)
	if total != 0 || journals != 3 || outbox != 3 || refunds != 2 {
		t.Fatalf("total %d journals %d outbox %d refunds %d", total, journals, outbox, refunds)
	}
}

func TestConcurrentPayTapsShareOneGatewayOrder(t *testing.T) {
	r := newRig(t)
	order, customer := uuid(21), uuid(22)
	r.completed(t, order, customer, uuid(23), 34900)
	ids := make([]string, 8)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := r.svc.CreateIntent(context.Background(), order, customer)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = p.GatewayOrderID
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("concurrent taps got different gateway orders: %v", ids)
		}
	}
}

func TestUnbalancedJournalIsRejectedByTheDatabase(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, r.svc.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO pay.journals (id, kind, ref, order_id) VALUES ($1, 'manual', 'x', $1)`, uuid(31)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO pay.ledger_entries (journal_id, account, debit_paise) VALUES ($1, 'asset:cash', 100)`, uuid(31))
		return err
	})
	if err == nil {
		t.Fatal("a one-sided journal committed")
	}
}
