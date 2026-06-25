package control

import (
	"errors"
	"testing"
	"time"
)

// fakeClock is an advanceable clock for billing tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }

func newSvc(t *testing.T) (*Service, *fakeClock) {
	t.Helper()
	store, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return NewService(store, clk.now, ManualProvider{}), clk
}

func TestQuotaEnforcement(t *testing.T) {
	svc, _ := newSvc(t)
	plan, err := svc.CreatePlan("Starter", 1000, 500, "EUR", PeriodMonthly)
	if err != nil {
		t.Fatal(err)
	}
	acc, token, err := svc.CreateAccount("Alice", "a@x.io", plan.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Token authenticates back to the account.
	got, err := svc.Authenticate(token)
	if err != nil || got.ID != acc.ID {
		t.Fatalf("authenticate: %v / %s", err, got.ID)
	}

	if err := svc.Reserve(acc.ID, 600); err != nil {
		t.Fatalf("reserve 600: %v", err)
	}
	// 600 + 500 = 1100 > 1000 quota: must be rejected.
	if err := svc.Reserve(acc.ID, 500); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	// Exactly to the limit is fine.
	if err := svc.Reserve(acc.ID, 400); err != nil {
		t.Fatalf("reserve to limit: %v", err)
	}
	st, _ := svc.Status(acc.ID)
	if st.BytesUsed != 1000 || st.RemainingBytes != 0 {
		t.Fatalf("status used=%d remaining=%d", st.BytesUsed, st.RemainingBytes)
	}
	// Release frees space again.
	if err := svc.Release(acc.ID, 400); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reserve(acc.ID, 100); err != nil {
		t.Fatalf("reserve after release: %v", err)
	}
}

func TestSuspendedAccountCannotStore(t *testing.T) {
	svc, _ := newSvc(t)
	plan, _ := svc.CreatePlan("P", 1000, 0, "EUR", PeriodMonthly)
	acc, _, _ := svc.CreateAccount("Bob", "", plan.ID)
	if err := svc.SetStatus(acc.ID, StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reserve(acc.ID, 10); !errors.Is(err, ErrSuspended) {
		t.Fatalf("expected suspended, got %v", err)
	}
}

func TestBillingCycleAndPayment(t *testing.T) {
	svc, clk := newSvc(t)
	plan, _ := svc.CreatePlan("Pro", 1<<30, 999, "EUR", PeriodMonthly)
	acc, _, _ := svc.CreateAccount("Carol", "c@x.io", plan.ID)

	// Before the period ends, no invoice.
	issued, err := svc.RunBilling()
	if err != nil {
		t.Fatal(err)
	}
	if len(issued) != 0 {
		t.Fatalf("billed too early: %d", len(issued))
	}

	// Advance past the period: one invoice for the plan price.
	clk.add(32 * 24 * time.Hour)
	issued, err = svc.RunBilling()
	if err != nil {
		t.Fatal(err)
	}
	if len(issued) != 1 || issued[0].AmountCents != 999 {
		t.Fatalf("unexpected invoices: %+v", issued)
	}
	st, _ := svc.Status(acc.ID)
	if st.OutstandingCts != 999 {
		t.Fatalf("outstanding = %d", st.OutstandingCts)
	}

	// Pay it; outstanding clears.
	if _, err := svc.PayInvoice(issued[0].ID, "card"); err != nil {
		t.Fatal(err)
	}
	st, _ = svc.Status(acc.ID)
	if st.OutstandingCts != 0 {
		t.Fatalf("still outstanding after payment: %d", st.OutstandingCts)
	}

	// Running billing again immediately must not double-bill.
	issued, _ = svc.RunBilling()
	if len(issued) != 0 {
		t.Fatalf("double billed: %d", len(issued))
	}
}

func TestDelinquencySuspendAndReactivate(t *testing.T) {
	svc, clk := newSvc(t)
	plan, _ := svc.CreatePlan("Pro", 1<<30, 1000, "EUR", PeriodMonthly)
	acc, _, _ := svc.CreateAccount("Dan", "d@x.io", plan.ID)

	clk.add(32 * 24 * time.Hour)
	issued, _ := svc.RunBilling()
	if len(issued) != 1 {
		t.Fatalf("expected 1 invoice")
	}
	// Move 10 days forward; enforce a 7-day grace -> suspended.
	clk.add(10 * 24 * time.Hour)
	susp := svc.EnforceDelinquency(7 * 24 * time.Hour)
	if len(susp) != 1 || susp[0] != acc.ID {
		t.Fatalf("expected suspension, got %v", susp)
	}
	if err := svc.Reserve(acc.ID, 1); !errors.Is(err, ErrSuspended) {
		t.Fatalf("suspended account stored anyway: %v", err)
	}
	// Pay -> reactivated.
	if _, err := svc.PayInvoice(issued[0].ID, "transfer"); err != nil {
		t.Fatal(err)
	}
	st, _ := svc.Status(acc.ID)
	if st.Status != StatusActive {
		t.Fatalf("not reactivated: %s", st.Status)
	}
}

func TestFilePersistenceRoundTrip(t *testing.T) {
	path := t.TempDir() + "/state.json"
	store, _ := NewStore(path)
	svc := NewService(store, time.Now, ManualProvider{})
	plan, _ := svc.CreatePlan("Persisted", 500, 100, "EUR", PeriodMonthly)
	acc, _, _ := svc.CreateAccount("Eve", "", plan.ID)
	_ = svc.Reserve(acc.ID, 123)

	// Reopen from disk; state must survive.
	store2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	svc2 := NewService(store2, time.Now, ManualProvider{})
	st, err := svc2.Status(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.BytesUsed != 123 || st.PlanName != "Persisted" {
		t.Fatalf("state not persisted: %+v", st)
	}
}

// autoProvider auto-collects, to verify the pluggable payment path.
type autoProvider struct{}

func (autoProvider) Name() string { return "auto" }
func (autoProvider) Charge(_ Account, inv Invoice) (Payment, error) {
	return Payment{AmountCents: inv.AmountCents, Method: "auto"}, nil
}

func TestAutoPaymentProvider(t *testing.T) {
	store, _ := NewStore("")
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	svc := NewService(store, clk.now, autoProvider{})
	plan, _ := svc.CreatePlan("Auto", 1<<30, 500, "EUR", PeriodMonthly)
	acc, _, _ := svc.CreateAccount("Frank", "", plan.ID)
	clk.add(32 * 24 * time.Hour)
	issued, _ := svc.RunBilling()
	if len(issued) != 1 || issued[0].Status != InvoicePaid {
		t.Fatalf("auto-charge failed: %+v", issued)
	}
	st, _ := svc.Status(acc.ID)
	if st.OutstandingCts != 0 {
		t.Fatalf("auto-paid invoice still outstanding: %d", st.OutstandingCts)
	}
}
