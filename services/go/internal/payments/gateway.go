package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GatewayOrder struct {
	ID     string `json:"id"`
	Amount int64  `json:"amount"`
	Status string `json:"status"`
}

type GatewayRefund struct {
	ID        string `json:"id"`
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
}

// Gateway is the slice of the Razorpay API we use. razorpay-local implements the same wire format.
type Gateway interface {
	CreateOrder(ctx context.Context, amountPaise int64, receipt string, notes map[string]string) (GatewayOrder, error)
	Refund(ctx context.Context, paymentID string, amountPaise int64, receipt string) (GatewayRefund, error)
	KeyID() string
}

type Razorpay struct {
	Base, Key, Secret string
	HTTP              *http.Client
}

func NewRazorpay(base, key, secret string) *Razorpay {
	return &Razorpay{Base: base, Key: key, Secret: secret, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (r *Razorpay) KeyID() string { return r.Key }

func (r *Razorpay) post(ctx context.Context, path string, body any, headers map[string]string, out any) error {
	js, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Base+path, bytes.NewReader(js))
	if err != nil {
		return err
	}
	req.SetBasicAuth(r.Key, r.Secret)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := r.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("razorpay %s: %d %s", path, res.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}

func (r *Razorpay) CreateOrder(ctx context.Context, amount int64, receipt string, notes map[string]string) (GatewayOrder, error) {
	var o GatewayOrder
	err := r.post(ctx, "/v1/orders", map[string]any{"amount": amount, "currency": "INR", "receipt": receipt, "notes": notes}, nil, &o)
	return o, err
}

// Refund sends our refund row id as the receipt and as Razorpay's refund idempotency header, so a
// retried call after a timeout cannot refund twice.
func (r *Razorpay) Refund(ctx context.Context, paymentID string, amount int64, receipt string) (GatewayRefund, error) {
	var f GatewayRefund
	err := r.post(ctx, "/v1/payments/"+paymentID+"/refund", map[string]any{"amount": amount, "receipt": receipt, "speed": "normal"},
		map[string]string{"X-Refund-Idempotency": receipt}, &f)
	return f, err
}

func hmacHex(secret string, msg []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(msg)
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyWebhook checks X-Razorpay-Signature: hex HMAC-SHA256 of the raw body with the webhook secret.
func VerifyWebhook(secret string, body []byte, signature string) bool {
	return secret != "" && hmac.Equal([]byte(hmacHex(secret, body)), []byte(signature))
}

// VerifyCheckout checks the signature the checkout SDK hands back: HMAC(order_id|payment_id, key secret).
func VerifyCheckout(keySecret, orderID, paymentID, signature string) bool {
	return hmac.Equal([]byte(hmacHex(keySecret, []byte(orderID+"|"+paymentID))), []byte(signature))
}

// SignWebhook and SignCheckout are used by razorpay-local and tests.
func SignWebhook(secret string, body []byte) string { return hmacHex(secret, body) }
func SignCheckout(keySecret, orderID, paymentID string) string {
	return hmacHex(keySecret, []byte(orderID+"|"+paymentID))
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
