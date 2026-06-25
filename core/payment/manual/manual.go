// Package manual is the offline/bank-transfer payment provider: it issues
// human instructions and is settled by an operator (vaultadmin invoice-pay).
// It is the default so the system is fully functional with no third party.
package manual

import (
	"context"
	"fmt"
	"net/http"

	"nftvault/payment"
)

// Provider implements payment.Provider for manual settlement.
type Provider struct {
	// Payee is shown to the customer (e.g. an IBAN or instructions line).
	Payee string
}

// New creates a manual provider.
func New(payee string) *Provider { return &Provider{Payee: payee} }

func (p *Provider) Name() string { return "manual" }

func (p *Provider) StartCheckout(_ context.Context, req payment.CheckoutRequest) (payment.CheckoutSession, error) {
	instr := fmt.Sprintf("Pay %.2f %s for invoice %s", float64(req.AmountCents)/100, req.Currency, req.InvoiceID)
	if p.Payee != "" {
		instr += " to " + p.Payee
	}
	return payment.CheckoutSession{
		Provider:     p.Name(),
		InvoiceID:    req.InvoiceID,
		Kind:         payment.KindManual,
		AmountCents:  req.AmountCents,
		Currency:     req.Currency,
		Instructions: instr,
	}, nil
}

// VerifyWebhook always reports not-paid: manual payments are confirmed by an
// operator, not by a callback.
func (p *Provider) VerifyWebhook(_ http.Header, _ []byte) (payment.WebhookResult, error) {
	return payment.WebhookResult{Provider: p.Name()}, nil
}
