package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"nftvault/payment"
)

func TestStartCheckoutHitsStripeAPI(t *testing.T) {
	var gotAuth, gotAmount, gotInvoice string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/checkout/sessions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		_ = r.ParseForm()
		gotAmount = r.Form.Get("line_items[0][price_data][unit_amount]")
		gotInvoice = r.Form.Get("metadata[invoice_id]")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"cs_test_123","url":"https://checkout.stripe.com/c/pay/cs_test_123"}`))
	}))
	defer mock.Close()

	p := New("sk_test_secret", "whsec_x", mock.URL)
	sess, err := p.StartCheckout(context.Background(), payment.CheckoutRequest{
		InvoiceID:   "inv_1",
		AccountID:   "acct_1",
		AmountCents: 499,
		Currency:    "EUR",
		Description: "10 GB plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk_test_secret" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotAmount != "499" {
		t.Fatalf("unit_amount = %q", gotAmount)
	}
	if gotInvoice != "inv_1" {
		t.Fatalf("invoice metadata = %q", gotInvoice)
	}
	if sess.Kind != payment.KindRedirect || !strings.Contains(sess.RedirectURL, "checkout.stripe.com") {
		t.Fatalf("unexpected session: %+v", sess)
	}
	if sess.Reference != "cs_test_123" {
		t.Fatalf("reference = %q", sess.Reference)
	}
}

func signed(secret, ts, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + body))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookValid(t *testing.T) {
	secret := "whsec_test"
	p := New("sk", secret, "")
	p.Now = func() time.Time { return time.Unix(1700000000, 0) }
	body := `{"type":"checkout.session.completed","data":{"object":{"id":"cs_1","payment_status":"paid","amount_total":499,"metadata":{"invoice_id":"inv_42"}}}}`
	ts := strconv.FormatInt(1700000000, 10)

	h := http.Header{}
	h.Set("Stripe-Signature", signed(secret, ts, body))
	res, err := p.VerifyWebhook(h, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Paid || res.InvoiceID != "inv_42" || res.AmountCents != 499 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestVerifyWebhookRejectsForgedSignature(t *testing.T) {
	p := New("sk", "whsec_real", "")
	p.Now = func() time.Time { return time.Unix(1700000000, 0) }
	body := `{"type":"checkout.session.completed","data":{"object":{"payment_status":"paid","metadata":{"invoice_id":"x"}}}}`
	h := http.Header{}
	h.Set("Stripe-Signature", signed("whsec_WRONG", "1700000000", body))
	if _, err := p.VerifyWebhook(h, []byte(body)); err == nil {
		t.Fatal("forged signature must be rejected")
	}
}

func TestVerifyWebhookRejectsOldTimestamp(t *testing.T) {
	secret := "whsec_test"
	p := New("sk", secret, "")
	p.Now = func() time.Time { return time.Unix(1700000000, 0) }
	old := strconv.FormatInt(1700000000-3600, 10) // 1h old, tolerance 5m
	body := `{"type":"checkout.session.completed","data":{"object":{"payment_status":"paid"}}}`
	h := http.Header{}
	h.Set("Stripe-Signature", signed(secret, old, body))
	if _, err := p.VerifyWebhook(h, []byte(body)); err == nil {
		t.Fatal("stale webhook must be rejected (replay protection)")
	}
}

func TestUnpaidStatusNotMarkedPaid(t *testing.T) {
	secret := "whsec_test"
	p := New("sk", secret, "")
	p.Now = func() time.Time { return time.Unix(1700000000, 0) }
	body := `{"type":"checkout.session.completed","data":{"object":{"payment_status":"unpaid","metadata":{"invoice_id":"inv_9"}}}}`
	h := http.Header{}
	h.Set("Stripe-Signature", signed(secret, "1700000000", body))
	res, err := p.VerifyWebhook(h, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if res.Paid {
		t.Fatal("unpaid session must not be marked paid")
	}
}
