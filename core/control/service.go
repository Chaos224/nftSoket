package control

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Clock returns the current time; injectable so billing logic is testable.
type Clock func() time.Time

// PaymentProvider attempts to collect money for an invoice. The default is
// manual (no auto-charge); a Stripe/other adapter can be slotted in later
// without touching the domain logic.
type PaymentProvider interface {
	Name() string
	// Charge returns a Payment if collected, or ErrManualPayment to leave the
	// invoice open for offline settlement.
	Charge(acc Account, inv Invoice) (Payment, error)
}

// ErrManualPayment signals "no automatic collection; settle manually".
var ErrManualPayment = errors.New("control: manual payment required")

// ManualProvider records nothing automatically; invoices stay open until paid.
type ManualProvider struct{}

func (ManualProvider) Name() string { return "manual" }
func (ManualProvider) Charge(Account, Invoice) (Payment, error) {
	return Payment{}, ErrManualPayment
}

// Service is the control-plane API over a Store.
type Service struct {
	mu      sync.Mutex
	store   *Store
	now     Clock
	payment PaymentProvider
}

// NewService wires a store; clock/provider default to real time / manual.
func NewService(store *Store, clock Clock, provider PaymentProvider) *Service {
	if clock == nil {
		clock = time.Now
	}
	if provider == nil {
		provider = ManualProvider{}
	}
	return &Service{store: store, now: clock, payment: provider}
}

func randID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// newToken returns a high-entropy API token and its lookup hash.
func newToken() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = "vlt_" + hex.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken is the deterministic lookup hash for an API token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// --- plans -----------------------------------------------------------------

// CreatePlan defines a subscription tier.
func (s *Service) CreatePlan(name string, quotaBytes int64, priceCents Money, currency string, period Period) (Plan, error) {
	if name == "" || quotaBytes <= 0 {
		return Plan{}, ErrInvalid
	}
	if currency == "" {
		currency = "EUR"
	}
	if period == "" {
		period = PeriodMonthly
	}
	p := Plan{ID: randID("plan"), Name: name, QuotaBytes: quotaBytes, PriceCents: priceCents, Currency: currency, Period: period}
	if err := s.store.SavePlan(p); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// ListPlans returns all plans.
func (s *Service) ListPlans() []Plan { return s.store.ListPlans() }

// --- accounts --------------------------------------------------------------

// CreateAccount creates a tenant on a plan and returns the one-time API token.
func (s *Service) CreateAccount(name, email, planID string) (Account, string, error) {
	if name == "" {
		return Account{}, "", ErrInvalid
	}
	if _, err := s.store.GetPlan(planID); err != nil {
		return Account{}, "", fmt.Errorf("plan: %w", err)
	}
	token, hash := newToken()
	now := s.now()
	a := Account{
		ID:          randID("acct"),
		Name:        name,
		Email:       email,
		PlanID:      planID,
		Status:      StatusActive,
		TokenHash:   hash,
		CreatedAt:   now,
		PeriodStart: now,
	}
	plan, _ := s.store.GetPlan(planID)
	a.PeriodEnd = plan.Period.advance(now)
	if err := s.store.SaveAccount(a); err != nil {
		return Account{}, "", err
	}
	return a, token, nil
}

// Authenticate resolves an API token to its account.
func (s *Service) Authenticate(token string) (Account, error) {
	if token == "" {
		return Account{}, ErrNotFound
	}
	return s.store.GetAccountByTokenHash(HashToken(token))
}

// SetStatus changes an account's status (suspend/activate/close).
func (s *Service) SetStatus(accountID string, status AccountStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.store.GetAccount(accountID)
	if err != nil {
		return err
	}
	a.Status = status
	return s.store.SaveAccount(a)
}

func (s *Service) ListAccounts() []Account { return s.store.ListAccounts() }

// --- space (quota) ---------------------------------------------------------

// Reserve accounts for `bytes` of newly stored data, enforcing the plan quota
// and that the account is active. It is the gate that "manages offered space".
func (s *Service) Reserve(accountID string, bytes int64) error {
	if bytes < 0 {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.store.GetAccount(accountID)
	if err != nil {
		return err
	}
	if a.Status != StatusActive {
		return ErrSuspended
	}
	plan, err := s.store.GetPlan(a.PlanID)
	if err != nil {
		return err
	}
	u := s.store.GetUsage(accountID)
	if u.BytesUsed+bytes > plan.QuotaBytes {
		return ErrQuotaExceeded
	}
	u.AccountID = accountID
	u.BytesUsed += bytes
	if bytes > 0 {
		u.ObjectCount++
	}
	u.UpdatedAt = s.now()
	return s.store.SaveUsage(u)
}

// Release returns previously reserved space (e.g. on delete).
func (s *Service) Release(accountID string, bytes int64) error {
	if bytes < 0 {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.store.GetUsage(accountID)
	u.AccountID = accountID
	u.BytesUsed -= bytes
	if u.BytesUsed < 0 {
		u.BytesUsed = 0
	}
	if bytes > 0 && u.ObjectCount > 0 {
		u.ObjectCount--
	}
	u.UpdatedAt = s.now()
	return s.store.SaveUsage(u)
}

// Status builds the read model for an account.
func (s *Service) Status(accountID string) (AccountStatusView, error) {
	a, err := s.store.GetAccount(accountID)
	if err != nil {
		return AccountStatusView{}, err
	}
	plan, err := s.store.GetPlan(a.PlanID)
	if err != nil {
		return AccountStatusView{}, err
	}
	u := s.store.GetUsage(accountID)
	remaining := plan.QuotaBytes - u.BytesUsed
	if remaining < 0 {
		remaining = 0
	}
	return AccountStatusView{
		AccountID:      a.ID,
		Name:           a.Name,
		Status:         a.Status,
		PlanID:         plan.ID,
		PlanName:       plan.Name,
		QuotaBytes:     plan.QuotaBytes,
		BytesUsed:      u.BytesUsed,
		RemainingBytes: remaining,
		ObjectCount:    u.ObjectCount,
		Currency:       plan.Currency,
		OutstandingCts: s.outstanding(accountID),
		PeriodEnd:      a.PeriodEnd,
	}, nil
}

// --- billing ---------------------------------------------------------------

func (s *Service) outstanding(accountID string) Money {
	var total Money
	for _, inv := range s.store.ListInvoices(accountID) {
		if inv.Status == InvoiceOpen {
			total += inv.AmountCents
		}
	}
	return total
}

// Invoices lists an account's invoices.
func (s *Service) Invoices(accountID string) []Invoice { return s.store.ListInvoices(accountID) }

// RunBilling issues invoices for every active account whose period has ended,
// advances the period, and asks the payment provider to collect. Idempotent per
// period: it only bills accounts whose PeriodEnd has passed.
func (s *Service) RunBilling() (issued []Invoice, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, a := range s.store.ListAccounts() {
		if a.Status == StatusClosed {
			continue
		}
		if now.Before(a.PeriodEnd) {
			continue
		}
		plan, perr := s.store.GetPlan(a.PlanID)
		if perr != nil {
			continue
		}
		if plan.PriceCents > 0 {
			inv := Invoice{
				ID:          randID("inv"),
				AccountID:   a.ID,
				PeriodStart: a.PeriodStart,
				PeriodEnd:   a.PeriodEnd,
				AmountCents: plan.PriceCents,
				Currency:    plan.Currency,
				Status:      InvoiceOpen,
				CreatedAt:   now,
			}
			// Attempt automatic collection.
			if pay, cerr := s.payment.Charge(a, inv); cerr == nil {
				pay.ID = randID("pay")
				pay.AccountID = a.ID
				pay.InvoiceID = inv.ID
				pay.CreatedAt = now
				paidAt := now
				inv.Status = InvoicePaid
				inv.PaidAt = &paidAt
				_ = s.store.SavePayment(pay)
			}
			if serr := s.store.SaveInvoice(inv); serr != nil {
				return issued, serr
			}
			issued = append(issued, inv)
		}
		// Advance the billing period.
		a.PeriodStart = a.PeriodEnd
		a.PeriodEnd = plan.Period.advance(a.PeriodEnd)
		if serr := s.store.SaveAccount(a); serr != nil {
			return issued, serr
		}
	}
	return issued, nil
}

// PayInvoice records a payment against an open invoice and reactivates a
// suspended account once it has no outstanding balance.
func (s *Service) PayInvoice(invoiceID, method string) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.store.GetInvoice(invoiceID)
	if err != nil {
		return Payment{}, err
	}
	if inv.Status != InvoiceOpen {
		return Payment{}, fmt.Errorf("%w: invoice not open", ErrInvalid)
	}
	now := s.now()
	inv.Status = InvoicePaid
	inv.PaidAt = &now
	if err := s.store.SaveInvoice(inv); err != nil {
		return Payment{}, err
	}
	pay := Payment{ID: randID("pay"), AccountID: inv.AccountID, InvoiceID: inv.ID, AmountCents: inv.AmountCents, Method: method, CreatedAt: now}
	if err := s.store.SavePayment(pay); err != nil {
		return Payment{}, err
	}
	// Reactivate if now settled.
	if s.outstanding(inv.AccountID) == 0 {
		if a, err := s.store.GetAccount(inv.AccountID); err == nil && a.Status == StatusSuspended {
			a.Status = StatusActive
			_ = s.store.SaveAccount(a)
		}
	}
	return pay, nil
}

// EnforceDelinquency suspends active accounts that have an open invoice older
// than grace. Run periodically alongside RunBilling.
func (s *Service) EnforceDelinquency(grace time.Duration) (suspended []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-grace)
	for _, a := range s.store.ListAccounts() {
		if a.Status != StatusActive {
			continue
		}
		for _, inv := range s.store.ListInvoices(a.ID) {
			if inv.Status == InvoiceOpen && inv.CreatedAt.Before(cutoff) {
				a.Status = StatusSuspended
				_ = s.store.SaveAccount(a)
				suspended = append(suspended, a.ID)
				break
			}
		}
	}
	return suspended
}
