package payment

import (
	"context"
	"net/http"
	"testing"
)

type stub struct{ name string }

func (s stub) Name() string { return s.name }
func (s stub) StartCheckout(context.Context, CheckoutRequest) (CheckoutSession, error) {
	return CheckoutSession{Provider: s.name}, nil
}
func (s stub) VerifyWebhook(http.Header, []byte) (WebhookResult, error) {
	return WebhookResult{Provider: s.name}, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(stub{"stripe"})
	r.Register(stub{"bitcoin"})

	if _, err := r.Get("stripe"); err != nil {
		t.Fatalf("get stripe: %v", err)
	}
	if _, err := r.Get("paypal"); err == nil {
		t.Fatal("unknown provider should error")
	}
	names := r.Names()
	if len(names) != 2 || names[0] != "bitcoin" || names[1] != "stripe" {
		t.Fatalf("names = %v (want sorted [bitcoin stripe])", names)
	}
}
