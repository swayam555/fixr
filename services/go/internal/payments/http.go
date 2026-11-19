package payments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"fixr.in/svc/internal/platform"
)

// Routes mounts the customer, ops and webhook endpoints. The webhook is unauthenticated by design:
// its HMAC signature is the authentication.
func (s *Service) Routes(mux *http.ServeMux, v *platform.Verifier) {
	mux.HandleFunc("POST /pay/webhook", s.WebhookHandler)
	mux.HandleFunc("POST /api/payments/orders/{orderId}", v.RequireRole("CUSTOMER", s.intentHandler))
	mux.HandleFunc("GET /api/payments/orders/{orderId}", v.RequireRole("CUSTOMER", s.statusHandler))
	mux.HandleFunc("POST /api/payments/confirm", v.RequireRole("CUSTOMER", s.confirmHandler))
	mux.HandleFunc("POST /api/payments/orders/{orderId}/refunds", v.RequireRole("OPS", s.refundHandler))
	mux.HandleFunc("GET /api/payments/ledger", v.RequireRole("OPS", s.ledgerHandler))
}

func (s *Service) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		platform.WriteError(w, http.StatusBadRequest, "BAD_BODY", "Unreadable body")
		return
	}
	if !VerifyWebhook(s.WebhookSecret, body, r.Header.Get("X-Razorpay-Signature")) {
		webhooksTotal.WithLabelValues("unknown", "bad_signature").Inc()
		platform.WriteError(w, http.StatusUnauthorized, "BAD_SIGNATURE", "Signature mismatch")
		return
	}
	eventID := r.Header.Get("X-Razorpay-Event-Id")
	if eventID == "" {
		sum := sha256.Sum256(body)
		eventID = "body:" + hex.EncodeToString(sum[:16])
	}
	applied, err := s.HandleWebhook(r.Context(), eventID, body)
	if err != nil {
		// a 5xx makes the gateway retry; the transaction rolled back so the retry starts clean
		slog.Error("webhook", "event", eventID, "err", err)
		platform.WriteError(w, http.StatusInternalServerError, "RETRY", "Not applied")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]bool{"applied": applied})
}

func (s *Service) intentHandler(w http.ResponseWriter, r *http.Request) {
	c := platform.ClaimsFrom(r.Context())
	p, err := s.CreateIntent(r.Context(), r.PathValue("orderId"), c.Subject)
	switch {
	case errors.Is(err, ErrNotBillable):
		platform.WriteError(w, http.StatusNotFound, "NOT_BILLABLE", "Payment opens once the job is marked complete")
	case errors.Is(err, ErrAlreadyPaid):
		platform.WriteJSON(w, http.StatusConflict, map[string]any{"code": "ALREADY_PAID", "message": "This job is already paid", "payment": p})
	case err != nil:
		slog.Error("create intent", "err", err)
		platform.WriteError(w, http.StatusBadGateway, "GATEWAY", "Could not start payment, try again")
	default:
		platform.WriteJSON(w, http.StatusOK, p)
	}
}

func (s *Service) statusHandler(w http.ResponseWriter, r *http.Request) {
	p, err := s.Status(r.Context(), r.PathValue("orderId"), platform.ClaimsFrom(r.Context()).Subject)
	if errors.Is(err, ErrPaymentAbsent) {
		platform.WriteError(w, http.StatusNotFound, "NO_PAYMENT", "No payment for this order")
		return
	}
	if err != nil {
		platform.WriteError(w, http.StatusInternalServerError, "INTERNAL", "Try again")
		return
	}
	platform.WriteJSON(w, http.StatusOK, p)
}

func (s *Service) confirmHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID   string `json:"razorpay_order_id"`
		PaymentID string `json:"razorpay_payment_id"`
		Signature string `json:"razorpay_signature"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil {
		platform.WriteError(w, http.StatusBadRequest, "BAD_BODY", "Expected checkout response")
		return
	}
	p, err := s.ConfirmCheckout(r.Context(), platform.ClaimsFrom(r.Context()).Subject, in.OrderID, in.PaymentID, in.Signature)
	switch {
	case errors.Is(err, ErrBadSignature):
		platform.WriteError(w, http.StatusBadRequest, "BAD_SIGNATURE", "Checkout response failed verification")
	case errors.Is(err, ErrPaymentAbsent):
		platform.WriteError(w, http.StatusNotFound, "NO_PAYMENT", "Unknown payment")
	case err != nil:
		platform.WriteError(w, http.StatusInternalServerError, "INTERNAL", "Try again")
	default:
		platform.WriteJSON(w, http.StatusOK, p)
	}
}

func (s *Service) refundHandler(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 64 {
		platform.WriteError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Send an Idempotency-Key header")
		return
	}
	var in struct {
		AmountPaise int64  `json:"amountPaise"`
		Reason      string `json:"reason"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in) != nil {
		platform.WriteError(w, http.StatusBadRequest, "BAD_BODY", "Expected amountPaise and reason")
		return
	}
	ref, err := s.RequestRefund(r.Context(), r.PathValue("orderId"), key, in.AmountPaise, in.Reason)
	switch {
	case errors.Is(err, ErrNotPaid):
		platform.WriteError(w, http.StatusConflict, "NOT_PAID", "Nothing captured to refund")
	case errors.Is(err, ErrRefundTooBig):
		platform.WriteError(w, http.StatusUnprocessableEntity, "REFUND_TOO_BIG", "Refund exceeds what is left on the payment")
	case err != nil:
		slog.Error("refund", "err", err)
		platform.WriteError(w, http.StatusBadGateway, "GATEWAY", "Refund was not accepted by the gateway")
	default:
		platform.WriteJSON(w, http.StatusAccepted, ref)
	}
}

func (s *Service) ledgerHandler(w http.ResponseWriter, r *http.Request) {
	bs, total, err := s.TrialBalance(r.Context(), r.URL.Query().Get("prefix"))
	if err != nil {
		platform.WriteError(w, http.StatusInternalServerError, "INTERNAL", "Try again")
		return
	}
	platform.WriteJSON(w, http.StatusOK, map[string]any{"accounts": bs, "totalPaise": total})
}
