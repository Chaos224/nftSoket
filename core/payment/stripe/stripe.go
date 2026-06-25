// Package stripe is the card-payment provider. It talks to Stripe's REST API
// directly over net/http (no SDK dependency, so the build stays small and
// vendored), creating a Checkout Session for "pay by card" and verifying signed
// webhooks. BaseURL is injectable so the provider is fully testable offline.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nftvault/payment"
)

// Provider implements payment.Provider for Stripe Checkout.
type Provider struct {
	secretKey     string
	webhookSecret string
	baseURL       string
	http          *http.Client
	// Tolerance, if > 0, rejects webhook timestamps older than this (replay
	// protection). Now is injectable for tests.
	Tolerance time.Duration
	Now       func() time.Time
}

// New builds a Stripe provider. baseURL defaults to the live API when empty.
func New(secretKey, webhookSecret, baseURL string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.stripe.com"
	}
	return &Provider{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		baseURL:       strings.TrimRight(baseURL, "/"),
		http:          &http.Client{Timeout: 20 * time.Second},
		Tolerance:     5 * time.Minute,
		Now:           time.Now,
	}
}

func (p *Provider) Name() string { return "stripe" }

// StartCheckout creates a Stripe Checkout Session and returns its hosted URL.
func (p *Provider) StartCheckout(ctx context.Context, req payment.CheckoutRequest) (payment.CheckoutSession, error) {
	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("success_url", orDefault(req.SuccessURL, "https://example.com/paid"))
	form.Set("cancel_url", orDefault(req.CancelURL, "https://example.com/cancel"))
	form.Set("client_reference_id", req.InvoiceID)
	form.Set("metadata[invoice_id]", req.InvoiceID)
	form.Set("metadata[account_id]", req.AccountID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(orDefault(req.Currency, "eur")))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(req.AmountCents, 10))
	form.Set("line_items[0][price_data][product_data][name]", orDefault(req.Description, "NFT Vault storage"))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return payment.CheckoutSession{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return payment.CheckoutSession{}, fmt.Errorf("stripe: checkout request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return payment.CheckoutSession{}, fmt.Errorf("stripe: checkout failed (%d): %s", resp.StatusCode, string(raw))
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return payment.CheckoutSession{}, fmt.Errorf("stripe: decode session: %w", err)
	}
	return payment.CheckoutSession{
		Provider:    p.Name(),
		InvoiceID:   req.InvoiceID,
		Kind:        payment.KindRedirect,
		RedirectURL: out.URL,
		Reference:   out.ID,
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
	}, nil
}

// VerifyWebhook authenticates a Stripe webhook using the Stripe-Signature header
// (scheme: "t=<unix>,v1=<hex hmac sha256 of 't.body'>") and reports payment.
func (p *Provider) VerifyWebhook(headers http.Header, body []byte) (payment.WebhookResult, error) {
	sig := headers.Get("Stripe-Signature")
	if sig == "" {
		return payment.WebhookResult{}, fmt.Errorf("stripe: missing Stripe-Signature")
	}
	ts, v1s := parseSigHeader(sig)
	if ts == "" || len(v1s) == 0 {
		return payment.WebhookResult{}, fmt.Errorf("stripe: malformed signature header")
	}
	if p.Tolerance > 0 {
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return payment.WebhookResult{}, fmt.Errorf("stripe: bad timestamp")
		}
		if diff := p.Now().Sub(time.Unix(n, 0)); diff > p.Tolerance || diff < -p.Tolerance {
			return payment.WebhookResult{}, fmt.Errorf("stripe: webhook timestamp outside tolerance")
		}
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write([]byte(ts + "." + string(body)))
	expected := hex.EncodeToString(mac.Sum(nil))
	ok := false
	for _, v := range v1s {
		if subtle.ConstantTimeCompare([]byte(v), []byte(expected)) == 1 {
			ok = true
			break
		}
	}
	if !ok {
		return payment.WebhookResult{}, fmt.Errorf("stripe: signature verification failed")
	}

	var ev struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				PaymentStatus     string            `json:"payment_status"`
				AmountTotal       int64             `json:"amount_total"`
				ClientReferenceID string            `json:"client_reference_id"`
				Metadata          map[string]string `json:"metadata"`
				ID                string            `json:"id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return payment.WebhookResult{}, fmt.Errorf("stripe: decode event: %w", err)
	}
	obj := ev.Data.Object
	invoiceID := obj.Metadata["invoice_id"]
	if invoiceID == "" {
		invoiceID = obj.ClientReferenceID
	}
	paid := ev.Type == "checkout.session.completed" && obj.PaymentStatus == "paid"
	return payment.WebhookResult{
		Provider:    p.Name(),
		InvoiceID:   invoiceID,
		Paid:        paid,
		Reference:   obj.ID,
		AmountCents: obj.AmountTotal,
	}, nil
}

func parseSigHeader(h string) (ts string, v1 []string) {
	for _, part := range strings.Split(h, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			v1 = append(v1, kv[1])
		}
	}
	return ts, v1
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
