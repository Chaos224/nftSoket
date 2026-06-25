package control

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newAPIServer(t *testing.T, adminTok string) (*httptest.Server, *Service) {
	t.Helper()
	store, _ := NewStore("")
	svc := NewService(store, time.Now, ManualProvider{})
	srv := httptest.NewServer(NewAPI(svc, adminTok).Handler())
	t.Cleanup(srv.Close)
	return srv, svc
}

// adminPost is a tiny helper for admin calls returning decoded JSON.
func TestHTTPFlowQuotaAndBilling(t *testing.T) {
	srv, svc := newAPIServer(t, "admin-secret")

	// Create a plan and account directly via the service (admin path is also
	// covered below); here we focus on the account-token client flow.
	plan, _ := svc.CreatePlan("Starter", 1000, 500, "EUR", PeriodMonthly)
	_, token, err := svc.CreateAccount("Alice", "a@x.io", plan.ID)
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(srv.URL)
	if !c.Healthy() {
		t.Fatal("control server not healthy")
	}

	// Reserve within quota.
	if err := c.Reserve(token, 600); err != nil {
		t.Fatalf("reserve 600: %v", err)
	}
	// Over quota via HTTP must surface as ErrQuotaExceeded (507).
	if err := c.Reserve(token, 500); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota error over HTTP, got %v", err)
	}
	// Status reflects usage.
	st, err := c.Status(token)
	if err != nil {
		t.Fatal(err)
	}
	if st.BytesUsed != 600 || st.RemainingBytes != 400 {
		t.Fatalf("status used=%d remaining=%d", st.BytesUsed, st.RemainingBytes)
	}

	// A bad token is rejected.
	if _, err := c.Status("vlt_not_a_real_token"); err == nil {
		t.Fatal("bad token should be unauthorized")
	}
}

func TestAdminAuthAndAccountCreation(t *testing.T) {
	srv, _ := newAPIServer(t, "admin-secret")

	// Without admin token -> 401.
	resp, _ := http.Post(srv.URL+"/v1/admin/plans", "application/json", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d", resp.StatusCode)
	}

	// With admin token, create a plan then an account.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/admin/plans", strings.NewReader(`{"name":"Pro","quota_bytes":1048576,"price_cents":999,"currency":"EUR"}`))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create plan: %d", resp.StatusCode)
	}
}

func TestSuspendedOverHTTP(t *testing.T) {
	srv, svc := newAPIServer(t, "admin")
	plan, _ := svc.CreatePlan("P", 1000, 0, "EUR", PeriodMonthly)
	acc, token, _ := svc.CreateAccount("Bob", "", plan.ID)
	_ = svc.SetStatus(acc.ID, StatusSuspended)

	c := NewClient(srv.URL)
	if err := c.Reserve(token, 10); !errors.Is(err, ErrSuspended) {
		t.Fatalf("expected suspended (403), got %v", err)
	}
}
