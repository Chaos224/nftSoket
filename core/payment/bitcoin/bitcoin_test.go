package bitcoin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"nftvault/payment"
)

// fixedRate is a test Rate: 1 EUR cent = 10 sats.
type fixedRate struct{}

func (fixedRate) ToSats(cents int64, _ string) (int64, error) { return cents * 10, nil }

func TestStartCheckoutBIP21(t *testing.T) {
	p := New(StaticAddress("bc1qexampleaddr"), fixedRate{}, "secret", 2)
	sess, err := p.StartCheckout(context.Background(), payment.CheckoutRequest{
		InvoiceID:   "inv_1",
		AmountCents: 499,
		Currency:    "EUR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Kind != payment.KindCrypto || sess.PayAddress != "bc1qexampleaddr" {
		t.Fatalf("unexpected session: %+v", sess)
	}
	// 499 cents * 10 sats = 4990 sats = 0.00004990 BTC
	if !strings.Contains(sess.PayURI, "bitcoin:bc1qexampleaddr?amount=0.00004990") {
		t.Fatalf("BIP21 URI = %q", sess.PayURI)
	}
}

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookConfirmations(t *testing.T) {
	p := New(StaticAddress("addr"), nil, "secret", 2)

	// 1 confirmation: not yet paid.
	body1 := `{"invoice_id":"inv_1","txid":"abc","confirmations":1,"amount_sat":4990}`
	h := http.Header{}
	h.Set("X-Signature", sign("secret", body1))
	res, err := p.VerifyWebhook(h, []byte(body1))
	if err != nil {
		t.Fatal(err)
	}
	if res.Paid {
		t.Fatal("1 confirmation should not be paid (min 2)")
	}

	// 2 confirmations: paid.
	body2 := `{"invoice_id":"inv_1","txid":"abc","confirmations":2,"amount_sat":4990}`
	h.Set("X-Signature", sign("secret", body2))
	res, err = p.VerifyWebhook(h, []byte(body2))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Paid || res.InvoiceID != "inv_1" || res.Reference != "abc" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestWebhookRejectsForgedSignature(t *testing.T) {
	p := New(StaticAddress("addr"), nil, "real-secret", 1)
	body := `{"invoice_id":"x","confirmations":9}`
	h := http.Header{}
	h.Set("X-Signature", sign("wrong-secret", body))
	if _, err := p.VerifyWebhook(h, []byte(body)); err == nil {
		t.Fatal("forged watcher signature must be rejected")
	}
}
