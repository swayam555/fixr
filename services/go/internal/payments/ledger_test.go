package payments

import "testing"

func sum(es []Entry) (d, c int64) {
	for _, e := range es {
		d += e.Debit
		c += e.Credit
	}
	return
}

func TestCaptureBalances(t *testing.T) {
	for _, amount := range []int64{100, 19900, 34901, 79999} {
		fee := FeeFor(amount, 2000)
		d, c := sum(CaptureEntries(amount, fee, "p1"))
		if d != c || d != amount {
			t.Fatalf("amount %d: debits %d credits %d", amount, d, c)
		}
	}
}

// Any sequence of partial refunds reverses the capture to the paisa, fee included.
func TestRefundsReverseCaptureExactly(t *testing.T) {
	cases := [][]int64{{19900}, {1, 19899}, {6633, 6633, 6634}, {3, 7, 11, 19879}}
	for _, parts := range cases {
		const amount = 19900
		fee := FeeFor(amount, 2000)
		balance := map[string]int64{}
		apply := func(es []Entry) {
			for _, e := range es {
				balance[e.Account] += e.Debit - e.Credit
			}
		}
		apply(CaptureEntries(amount, fee, "p1"))
		var refunded, feeRefunded int64
		for _, r := range parts {
			fp := RefundSplit(amount, fee, refunded, feeRefunded, r)
			if fp < 0 || fp > r {
				t.Fatalf("%v: fee part %d out of range for refund %d", parts, fp, r)
			}
			es := RefundEntries(r, fp, "p1")
			if d, c := sum(es); d != c {
				t.Fatalf("%v: unbalanced refund journal", parts)
			}
			apply(es)
			refunded += r
			feeRefunded += fp
		}
		for acc, b := range balance {
			if b != 0 {
				t.Fatalf("%v: %s left at %d after full refund", parts, acc, b)
			}
		}
	}
}

func TestSignatures(t *testing.T) {
	body := []byte(`{"event":"payment.captured"}`)
	sig := SignWebhook("whsec", body)
	if !VerifyWebhook("whsec", body, sig) {
		t.Fatal("valid webhook rejected")
	}
	if VerifyWebhook("whsec", append(body, ' '), sig) || VerifyWebhook("other", body, sig) || VerifyWebhook("", body, SignWebhook("", body)) {
		t.Fatal("tampered body, wrong secret or empty secret accepted")
	}
	cs := SignCheckout("ksec", "order_1", "pay_1")
	if !VerifyCheckout("ksec", "order_1", "pay_1", cs) || VerifyCheckout("ksec", "order_1", "pay_2", cs) {
		t.Fatal("checkout signature check wrong")
	}
}
