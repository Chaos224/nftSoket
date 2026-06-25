// Package payment is the payment middleware: a single Provider abstraction plus
// a Registry, so payment methods are pluggable. Card payments (Stripe), crypto
// (Bitcoin), and manual settlement each live in their own subpackage and are
// wired in at the composition root (cmd/vaultserver) — nothing in the domain or
// control plane depends on a concrete provider, so methods can be added or
// improved zone by zone without touching the rest.
//
// This package deliberately depends on nothing in the project (no control,
// no SDKs): its types use primitive fields only, so there is no import cycle and
// providers stay independently testable.
package payment

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Kind describes how a payer completes a checkout.
type Kind string

const (
	KindRedirect Kind = "redirect" // hosted page (e.g. Stripe Checkout URL)
	KindCrypto   Kind = "crypto"   // on-chain address + URI (e.g. Bitcoin)
	KindManual   Kind = "manual"   // offline / bank transfer, settled by an operator
)

// CheckoutRequest is the provider-agnostic request to collect money for one
// invoice. Amounts are integer minor units (cents / smallest fiat unit).
type CheckoutRequest struct {
	InvoiceID   string
	AccountID   string
	AmountCents int64
	Currency    string // ISO 4217, e.g. "EUR"
	Description string
	SuccessURL  string // where to send the payer after a hosted checkout
	CancelURL   string
}

// CheckoutSession is what the payer needs to complete payment.
type CheckoutSession struct {
	Provider    string    `json:"provider"`
	InvoiceID   string    `json:"invoice_id"`
	Kind        Kind      `json:"kind"`
	RedirectURL string    `json:"redirect_url,omitempty"` // KindRedirect
	PayAddress  string    `json:"pay_address,omitempty"`  // KindCrypto
	PayURI      string    `json:"pay_uri,omitempty"`      // KindCrypto (e.g. bitcoin:...)
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Reference   string    `json:"reference,omitempty"` // provider-side session/tx id
	Instructions string   `json:"instructions,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

// WebhookResult is the normalized outcome of a provider callback.
type WebhookResult struct {
	Provider    string
	InvoiceID   string
	Paid        bool
	Reference   string
	AmountCents int64
}

// Provider is the single interface every payment method implements.
type Provider interface {
	// Name is the stable identifier used in URLs and config (e.g. "stripe").
	Name() string
	// StartCheckout creates a payment session for an invoice.
	StartCheckout(ctx context.Context, req CheckoutRequest) (CheckoutSession, error)
	// VerifyWebhook authenticates an inbound provider callback (the request's
	// signature header(s) plus raw body) and reports whether an invoice is paid.
	// It MUST reject unauthenticated/forged callbacks.
	VerifyWebhook(headers http.Header, body []byte) (WebhookResult, error)
}

// ErrUnknownProvider is returned when a name is not registered.
var ErrUnknownProvider = errors.New("payment: unknown provider")

// Registry holds the enabled providers. It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

// Register adds (or replaces) a provider.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get resolves a provider by name.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return p, nil
}

// Names lists registered provider names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for n := range r.providers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
