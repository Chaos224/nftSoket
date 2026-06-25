package control

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"nftvault/payment"
	"nftvault/payment/stripe"
)

// TestStripeCheckoutAndWebhookSettlesInvoice exercises the whole payment
// middleware path through the control service: create an invoice, start a Stripe
// checkout (against a mock Stripe API), then deliver a signed webhook and verify
// the invoice is marked paid.
func TestStripeCheckoutAndWebhookSettlesInvoice(t *testing.T) {
	// Mock Stripe API returning a checkout session.
	stripeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"cs_live_1","url":"https://checkout.stripe.com/c/pay/cs_live_1"}`))
	}))
	defer stripeAPI.Close()

	const whSecret = "whsec_integration"
	sp := stripe.New("sk_test", whSecret, stripeAPI.URL)
	fixedNow := time.Unix(1700000000, 0)
	sp.Now = func() time.Time { return fixedNow }

	reg := payment.NewRegistry()
	reg.Register(sp)

	store, _ := NewStore("")
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	svc := NewService(store, clk.now, ManualProvider{}).WithPayments(reg)

	plan, _ := svc.CreatePlan("10GB", 10<<30, 499, "EUR", PeriodMonthly)
	acc, _, _ := svc.CreateAccount("Alice", "a@x.io", plan.ID)

	// Generate an invoice by running billing after the period.
	clk.add(32 * 24 * time.Hour)
	issued, err := svc.RunBilling()
	if err != nil || len(issued) != 1 {
		t.Fatalf("billing: %v issued=%d", err, len(issued))
	}
	inv := issued[0]

	// Start a Stripe checkout for the invoice.
	sess, err := svc.CreateCheckout(context.Background(), inv.ID, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Kind != payment.KindRedirect || sess.RedirectURL == "" {
		t.Fatalf("unexpected checkout session: %+v", sess)
	}

	// Before the webhook, the invoice is still outstanding.
	st, _ := svc.Status(acc.ID)
	if st.OutstandingCts != 499 {
		t.Fatalf("pre-webhook outstanding = %d", st.OutstandingCts)
	}

	// Deliver a signed Stripe webhook for this invoice.
	body := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_live_1","payment_status":"paid","amount_total":499,"metadata":{"invoice_id":"` + inv.ID + `"}}}}`
	ts := strconv.FormatInt(fixedNow.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(whSecret))
	mac.Write([]byte(ts + "." + body))
	h := http.Header{}
	h.Set("Stripe-Signature", "t="+ts+",v1="+hex.EncodeToString(mac.Sum(nil)))

	settled, err := svc.HandleWebhook("stripe", h, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !settled {
		t.Fatal("webhook did not settle the invoice")
	}
	st, _ = svc.Status(acc.ID)
	if st.OutstandingCts != 0 {
		t.Fatalf("post-webhook outstanding = %d (want 0)", st.OutstandingCts)
	}

	// A forged webhook must be rejected and must not settle anything.
	forged := http.Header{}
	forged.Set("Stripe-Signature", "t="+ts+",v1=deadbeef")
	if _, err := svc.HandleWebhook("stripe", forged, []byte(body)); err == nil {
		t.Fatal("forged webhook should be rejected")
	}
}

func TestCheckoutUnknownProvider(t *testing.T) {
	store, _ := NewStore("")
	svc := NewService(store, time.Now, ManualProvider{}).WithPayments(payment.NewRegistry())
	plan, _ := svc.CreatePlan("P", 1<<30, 100, "EUR", PeriodMonthly)
	_, _, _ = svc.CreateAccount("Bob", "", plan.ID)
	if _, err := svc.CreateCheckout(context.Background(), "inv_missing", "stripe", "", ""); err == nil {
		t.Fatal("expected error for unknown provider / missing invoice")
	}
}
