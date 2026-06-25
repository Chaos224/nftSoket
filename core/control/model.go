// Package control is the self-hosted control server's domain: accounts, plans,
// the space (quota) they are offered and consume, and billing (subscriptions,
// invoices, payments). It has no external dependencies — persistence is an
// optional local JSON file, and payment is a pluggable interface that defaults
// to a manual/offline ledger — so the whole thing stays self-owned.
package control

import (
	"errors"
	"time"
)

// Money is stored as integer minor units (cents) to avoid float rounding.
type Money = int64

// Period is a billing cadence.
type Period string

const PeriodMonthly Period = "monthly"

func (p Period) advance(t time.Time) time.Time {
	switch p {
	case PeriodMonthly:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 1, 0)
	}
}

// Plan is a subscription tier: how much space it offers and what it costs.
type Plan struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	QuotaBytes int64  `json:"quota_bytes"`
	PriceCents Money  `json:"price_cents"`
	Currency   string `json:"currency"`
	Period     Period `json:"period"`
}

// AccountStatus controls whether an account may store data.
type AccountStatus string

const (
	StatusActive    AccountStatus = "active"
	StatusSuspended AccountStatus = "suspended" // e.g. unpaid past grace
	StatusClosed    AccountStatus = "closed"
)

// Account is a billed tenant. The API token is never stored in clear; only
// TokenHash (sha256 hex) is kept, and it is what we look accounts up by.
type Account struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Email       string        `json:"email"`
	PlanID      string        `json:"plan_id"`
	Status      AccountStatus `json:"status"`
	TokenHash   string        `json:"token_hash"`
	CreatedAt   time.Time     `json:"created_at"`
	PeriodStart time.Time     `json:"period_start"`
	PeriodEnd   time.Time     `json:"period_end"`
}

// Usage is the live space accounting for an account.
type Usage struct {
	AccountID   string    `json:"account_id"`
	BytesUsed   int64     `json:"bytes_used"`
	ObjectCount int64     `json:"object_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// InvoiceStatus is the lifecycle of an invoice.
type InvoiceStatus string

const (
	InvoiceOpen InvoiceStatus = "open"
	InvoicePaid InvoiceStatus = "paid"
	InvoiceVoid InvoiceStatus = "void"
)

// Invoice is one period's charge for an account.
type Invoice struct {
	ID          string        `json:"id"`
	AccountID   string        `json:"account_id"`
	PeriodStart time.Time     `json:"period_start"`
	PeriodEnd   time.Time     `json:"period_end"`
	AmountCents Money         `json:"amount_cents"`
	Currency    string        `json:"currency"`
	Status      InvoiceStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	PaidAt      *time.Time    `json:"paid_at,omitempty"`
	// Payment middleware traceability (forward-compatible, optional).
	CheckoutProvider string `json:"checkout_provider,omitempty"`
	CheckoutRef      string `json:"checkout_ref,omitempty"`
}

// Payment records money received against an invoice.
type Payment struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	InvoiceID   string    `json:"invoice_id"`
	AmountCents Money     `json:"amount_cents"`
	Method      string    `json:"method"`
	CreatedAt   time.Time `json:"created_at"`
}

// Domain errors.
var (
	ErrNotFound      = errors.New("control: not found")
	ErrQuotaExceeded = errors.New("control: storage quota exceeded")
	ErrSuspended     = errors.New("control: account not active")
	ErrConflict      = errors.New("control: already exists")
	ErrInvalid       = errors.New("control: invalid argument")
)

// AccountStatusView is the read model returned to clients (no secrets).
type AccountStatusView struct {
	AccountID      string        `json:"account_id"`
	Name           string        `json:"name"`
	Status         AccountStatus `json:"status"`
	PlanID         string        `json:"plan_id"`
	PlanName       string        `json:"plan_name"`
	QuotaBytes     int64         `json:"quota_bytes"`
	BytesUsed      int64         `json:"bytes_used"`
	RemainingBytes int64         `json:"remaining_bytes"`
	ObjectCount    int64         `json:"object_count"`
	Currency       string        `json:"currency"`
	OutstandingCts Money         `json:"outstanding_cents"`
	PeriodEnd      time.Time     `json:"period_end"`
}
