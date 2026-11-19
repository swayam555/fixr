package payments

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Accounts. Balances are debit-positive: an asset carries a positive balance; liabilities and
// income carry a negative one. All amounts are paise.
const (
	AccGatewayClearing = "asset:gateway_clearing"
	AccPlatformRevenue = "income:platform_fee"
	accProPayable      = "liability:pro_payable:"
)

func AccProPayable(proID string) string { return accProPayable + proID }

type Entry struct {
	Account string
	Debit   int64
	Credit  int64
}

// FeeFor is the platform's cut of a captured amount, rounded down to the paisa.
func FeeFor(amount int64, feeBps int) int64 { return amount * int64(feeBps) / 10_000 }

// CaptureEntries: money lands at the gateway; the pro is owed their share and we book our fee.
func CaptureEntries(amount, fee int64, proID string) []Entry {
	es := []Entry{{Account: AccGatewayClearing, Debit: amount}, {Account: AccProPayable(proID), Credit: amount - fee}}
	if fee > 0 {
		es = append(es, Entry{Account: AccPlatformRevenue, Credit: fee})
	}
	return es
}

// RefundSplit divides a refund between the pro's share and our fee in the same ratio as the capture.
// The refund that brings the total to the full amount takes the remainder, so a payment refunded in
// any number of parts reverses its capture exactly.
func RefundSplit(amount, fee, refundedSoFar, feeRefundedSoFar, refund int64) (feePart int64) {
	if refundedSoFar+refund == amount {
		return fee - feeRefundedSoFar
	}
	return refund * fee / amount
}

func RefundEntries(refund, feePart int64, proID string) []Entry {
	es := []Entry{{Account: AccProPayable(proID), Debit: refund - feePart}, {Account: AccGatewayClearing, Credit: refund}}
	if feePart > 0 {
		es = append(es, Entry{Account: AccPlatformRevenue, Debit: feePart})
	}
	return es
}

var ErrUnbalanced = errors.New("journal does not balance")

// postJournal writes a journal once per (kind, ref). It returns false when that journal already
// exists, which is how a replayed event becomes a no-op even if it slipped past the event-id dedupe.
func postJournal(ctx context.Context, tx pgx.Tx, kind, ref, orderID string, entries []Entry) (bool, error) {
	var sum int64
	for _, e := range entries {
		sum += e.Debit - e.Credit
	}
	if sum != 0 {
		return false, ErrUnbalanced
	}
	id := newUUID()
	tag, err := tx.Exec(ctx, `INSERT INTO pay.journals (id, kind, ref, order_id) VALUES ($1, $2, $3, $4)
		ON CONFLICT (kind, ref) DO NOTHING`, id, kind, ref, orderID)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	for _, e := range entries {
		if e.Debit == 0 && e.Credit == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO pay.ledger_entries (journal_id, account, debit_paise, credit_paise)
			VALUES ($1, $2, $3, $4)`, id, e.Account, e.Debit, e.Credit); err != nil {
			return false, err
		}
	}
	ledgerJournals.WithLabelValues(kind).Inc()
	return true, nil
}
