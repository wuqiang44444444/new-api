package model

import "errors"

// Engineering retained-evidence plus current-page budget, not cumulative bytes
// already processed and released. Exceeding
// one aborts the entire review, so no partial fingerprint can authorize a month.
var errBillingSourceReviewBudget = errors.New("source review evidence budget exceeded; no review produced")

const billingSourceReviewMaxDependencies = 20000

type billingSourceReviewBudget struct{ remaining int }

func (b *billingSourceReviewBudget) consume(bytes int) error {
	if bytes < 0 || bytes > b.remaining {
		return errBillingSourceReviewBudget
	}
	b.remaining -= bytes
	return nil
}

// Cross-period links retain only the fields used by reconciliation/fingerprints.
// Content, token labels and all unrelated log fields remain in the current page.
type billingSourceRelatedEvidence struct {
	ID, TokenID, ChannelID, Type, Quota int
	CreatedAt                           int64
	Model, Other                        string
}
