// Package rzplocal is a stand-in for Razorpay's Orders/Refunds API and its signed webhooks, so the
// whole payment flow (including duplicate and out-of-order deliveries) runs offline.
package rzplocal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"fixr.in/svc/internal/payments"
	"fixr.in/svc/internal/platform"
)

type order struct {
	ID       string            `json:"id"`
	Entity   string            `json:"entity"`
	Amount   int64             `json:"amount"`
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Status   string            `json:"status"`
	Notes    map[string]string `json:"notes"`
}

type payment struct {
	ID       string `json:"id"`
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Refunded int64  `json:"amount_refunded"`
	Method   string `json:"method"`
	Status   string `json:"status"`
}

type sent struct {
	EventID string          `json:"eventId"`
	Event   string          `json:"event"`
	Body    json.RawMessage `json:"body"`
	Status  int             `json:"status"`
}

type Server struct {
	Key, Secret, WebhookSecret, WebhookURL string
	// Duplicate sends every webhook twice, the way the real gateway can on a slow ack.
	Duplicate bool
	Delay     time.Duration
	HTTP      *http.Client

	mu       sync.Mutex
	orders   map[string]*order
	payments map[string]*payment
	refunds  map[string]string // receipt (our refund id) -> refund id, for idempotent retries
	log      []sent
}

func New(key, secret, webhookSecret, webhookURL string) *Server {
	return &Server{Key: key, Secret: secret, WebhookSecret: webhookSecret, WebhookURL: webhookURL,
		Delay: 300 * time.Millisecond, HTTP: &http.Client{Timeout: 5 * time.Second},
		orders: map[string]*order{}, payments: map[string]*payment{}, refunds: map[string]string{}}
}

func id(prefix string) string {
	b := make([]byte, 7)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/orders", s.auth(s.createOrder))
	mux.HandleFunc("GET /v1/orders/{id}", s.auth(s.getOrder))
	mux.HandleFunc("POST /v1/payments/{id}/refund", s.auth(s.refund))
	mux.HandleFunc("POST /_local/checkout", s.checkout)
	mux.HandleFunc("GET /_local/webhooks", s.listSent)
	mux.HandleFunc("POST /_local/webhooks/{eventId}/replay", s.replay)
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if k, sec, ok := r.BasicAuth(); !ok || k != s.Key || sec != s.Secret {
			platform.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{
				"code": "BAD_REQUEST_ERROR", "description": "The api key provided is invalid"}})
			return
		}
		next(w, r)
	}
}

func badRequest(w http.ResponseWriter, msg string) {
	platform.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "BAD_REQUEST_ERROR", "description": msg}})
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	var in order
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Amount < 100 {
		badRequest(w, "amount must be at least 100 paise")
		return
	}
	o := &order{ID: id("order"), Entity: "order", Amount: in.Amount, Currency: "INR", Receipt: in.Receipt, Status: "created", Notes: in.Notes}
	s.mu.Lock()
	s.orders[o.ID] = o
	s.mu.Unlock()
	platform.WriteJSON(w, http.StatusOK, o)
}

func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	o, ok := s.orders[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		badRequest(w, "The id provided does not exist")
		return
	}
	platform.WriteJSON(w, http.StatusOK, o)
}

// checkout plays the customer's side of the checkout sheet: it "pays" (or fails) a gateway order and
// returns what the real SDK hands the app, then fires the webhook a moment later.
func (s *Server) checkout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID string `json:"order_id"`
		Method  string `json:"method"`
		Fail    bool   `json:"fail"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil {
		badRequest(w, "order_id required")
		return
	}
	if in.Method == "" {
		in.Method = "upi"
	}
	s.mu.Lock()
	o, ok := s.orders[in.OrderID]
	if !ok || o.Status == "paid" {
		s.mu.Unlock()
		badRequest(w, "order not found or already paid")
		return
	}
	p := &payment{ID: id("pay"), OrderID: o.ID, Amount: o.Amount, Method: in.Method, Status: "captured"}
	event := "payment.captured"
	if in.Fail {
		p.Status, event = "failed", "payment.failed"
	} else {
		o.Status = "paid"
	}
	s.payments[p.ID] = p
	snapshot := *p
	s.mu.Unlock()

	go s.fire(event, map[string]any{"payment": map[string]any{"entity": snapshot}})
	platform.WriteJSON(w, http.StatusOK, map[string]string{
		"razorpay_order_id": o.ID, "razorpay_payment_id": p.ID,
		"razorpay_signature": payments.SignCheckout(s.Secret, o.ID, p.ID)})
}

func (s *Server) refund(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Amount  int64  `json:"amount"`
		Receipt string `json:"receipt"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Amount <= 0 {
		badRequest(w, "amount required")
		return
	}
	idem := r.Header.Get("X-Refund-Idempotency")
	s.mu.Lock()
	p, ok := s.payments[r.PathValue("id")]
	if !ok || p.Status != "captured" {
		s.mu.Unlock()
		badRequest(w, "payment not captured")
		return
	}
	if prev, seen := s.refunds[idem]; idem != "" && seen {
		s.mu.Unlock()
		platform.WriteJSON(w, http.StatusOK, map[string]any{"id": prev, "payment_id": p.ID, "amount": in.Amount, "status": "pending"})
		return
	}
	if p.Refunded+in.Amount > p.Amount {
		s.mu.Unlock()
		badRequest(w, "The refund amount provided is greater than amount captured")
		return
	}
	p.Refunded += in.Amount
	rf := map[string]any{"id": id("rfnd"), "entity": "refund", "payment_id": p.ID, "amount": in.Amount,
		"currency": "INR", "receipt": in.Receipt, "status": "processed"}
	if idem != "" {
		s.refunds[idem] = rf["id"].(string)
	}
	s.mu.Unlock()

	go s.fire("refund.processed", map[string]any{"refund": map[string]any{"entity": rf}})
	pending := map[string]any{}
	for k, v := range rf {
		pending[k] = v
	}
	pending["status"] = "pending"
	platform.WriteJSON(w, http.StatusOK, pending)
}

func (s *Server) fire(event string, payload map[string]any) {
	time.Sleep(s.Delay)
	entity, _, _ := strings.Cut(event, ".")
	body, _ := json.Marshal(map[string]any{"entity": "event", "account_id": "acc_local", "event": event,
		"contains": []string{entity}, "payload": payload, "created_at": time.Now().Unix()})
	eventID := id("evt")
	times := 1
	if s.Duplicate {
		times = 2
	}
	for range times {
		s.deliver(eventID, event, body)
	}
}

func (s *Server) deliver(eventID, event string, body []byte) int {
	status := 0
	for attempt := range 5 {
		req, _ := http.NewRequest(http.MethodPost, s.WebhookURL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Razorpay-Event-Id", eventID)
		req.Header.Set("X-Razorpay-Signature", payments.SignWebhook(s.WebhookSecret, body))
		res, err := s.HTTP.Do(req)
		if err == nil {
			status = res.StatusCode
			res.Body.Close()
			if status < 300 {
				break
			}
		}
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	slog.Info("webhook", "event", event, "id", eventID, "status", status)
	s.mu.Lock()
	s.log = append(s.log, sent{EventID: eventID, Event: event, Body: body, Status: status})
	s.mu.Unlock()
	return status
}

func (s *Server) listSent(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	platform.WriteJSON(w, http.StatusOK, s.log)
}

// replay re-sends a webhook byte for byte, with the same event id and signature.
func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	var found *sent
	s.mu.Lock()
	for i := range s.log {
		if s.log[i].EventID == r.PathValue("eventId") {
			found = &s.log[i]
			break
		}
	}
	s.mu.Unlock()
	if found == nil {
		badRequest(w, "no such event")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]int{"status": s.deliver(found.EventID, found.Event, found.Body)})
}
