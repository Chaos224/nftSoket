// Package bitcoin is the crypto payment provider (future-facing but structured
// so it can be switched on without touching the rest of the system).
//
// StartCheckout hands the payer a Bitcoin address and a BIP21 URI. Confirmation
// is delivered by a chain watcher that POSTs an HMAC-signed callback once the
// transaction reaches the required confirmations — VerifyWebhook authenticates
// it. The address source and the fiat→BTC rate are interfaces, so a real
// deployment plugs in an xpub-derived address pool and a live price feed without
// changing this provider's logic.
package bitcoin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nftvault/payment"
)

// AddressSource yields a receiving address for an invoice. Implementations may
// derive per-invoice addresses from an xpub for privacy.
type AddressSource interface {
	NextAddress(invoiceID string) (string, error)
}

// StaticAddress is a trivial AddressSource that always returns one address.
// Fine for testing; use a derived source in production.
type StaticAddress string

func (s StaticAddress) NextAddress(string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("bitcoin: no address configured")
	}
	return string(s), nil
}

// Rate converts fiat minor units to satoshis. Plug in a live feed in production.
type Rate interface {
	ToSats(amountCents int64, currency string) (int64, error)
}

// Provider implements payment.Provider for on-chain Bitcoin.
type Provider struct {
	addresses     AddressSource
	rate          Rate // optional; if nil the URI omits the amount
	webhookSecret string
	minConf       int
}

// New builds a Bitcoin provider. minConfirmations is how many confirmations a
// watcher must report before an invoice counts as paid (commonly 1–3).
func New(addresses AddressSource, rate Rate, webhookSecret string, minConfirmations int) *Provider {
	if minConfirmations < 1 {
		minConfirmations = 1
	}
	return &Provider{addresses: addresses, rate: rate, webhookSecret: webhookSecret, minConf: minConfirmations}
}

func (p *Provider) Name() string { return "bitcoin" }

// StartCheckout returns a payment address and BIP21 URI for the invoice.
func (p *Provider) StartCheckout(_ context.Context, req payment.CheckoutRequest) (payment.CheckoutSession, error) {
	addr, err := p.addresses.NextAddress(req.InvoiceID)
	if err != nil {
		return payment.CheckoutSession{}, err
	}
	uri := "bitcoin:" + addr
	params := []string{}
	if p.rate != nil {
		sats, rerr := p.rate.ToSats(req.AmountCents, req.Currency)
		if rerr != nil {
			return payment.CheckoutSession{}, rerr
		}
		// BIP21 amount is in BTC.
		btc := float64(sats) / 1e8
		params = append(params, "amount="+strconv.FormatFloat(btc, 'f', 8, 64))
	}
	params = append(params, "label="+urlEscape("NFT Vault invoice "+req.InvoiceID))
	uri += "?" + strings.Join(params, "&")

	instr := fmt.Sprintf("Send the exact amount to %s. Payment confirms after %d block confirmation(s).", addr, p.minConf)
	return payment.CheckoutSession{
		Provider:     p.Name(),
		InvoiceID:    req.InvoiceID,
		Kind:         payment.KindCrypto,
		PayAddress:   addr,
		PayURI:       uri,
		AmountCents:  req.AmountCents,
		Currency:     req.Currency,
		Instructions: instr,
	}, nil
}

// watcherEvent is the signed callback a chain watcher sends.
type watcherEvent struct {
	InvoiceID     string `json:"invoice_id"`
	TxID          string `json:"txid"`
	Confirmations int    `json:"confirmations"`
	AmountSat     int64  `json:"amount_sat"`
}

// VerifyWebhook authenticates a watcher callback via the X-Signature header
// (hex HMAC-SHA256 of the raw body) and reports paid once confirmations suffice.
func (p *Provider) VerifyWebhook(headers http.Header, body []byte) (payment.WebhookResult, error) {
	sig := headers.Get("X-Signature")
	if sig == "" {
		return payment.WebhookResult{}, fmt.Errorf("bitcoin: missing X-Signature")
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return payment.WebhookResult{}, fmt.Errorf("bitcoin: signature verification failed")
	}
	var ev watcherEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return payment.WebhookResult{}, fmt.Errorf("bitcoin: decode event: %w", err)
	}
	return payment.WebhookResult{
		Provider:    p.Name(),
		InvoiceID:   ev.InvoiceID,
		Paid:        ev.Confirmations >= p.minConf,
		Reference:   ev.TxID,
		AmountCents: 0, // settled in BTC; fiat amount tracked on the invoice
	}, nil
}

func urlEscape(s string) string {
	return strings.ReplaceAll(s, " ", "%20")
}
