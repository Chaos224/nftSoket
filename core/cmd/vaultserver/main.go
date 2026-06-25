// Command vaultserver runs the self-hosted control plane: accounts, plans,
// storage quota ("offered space") enforcement, and billing. State persists to a
// local JSON file — no external database. Payment defaults to a manual/offline
// ledger; a payment provider can be slotted in later without code changes here.
//
//	NFTVAULT_ADMIN_TOKEN=... vaultserver --listen :9000 --state ./control.json
//
// Pass --tls-cert/--tls-key for HTTPS. Admin routes require the admin token.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"

	"nftvault/control"
	"nftvault/payment"
	"nftvault/payment/bitcoin"
	"nftvault/payment/manual"
	"nftvault/payment/stripe"
)

// buildPayments assembles the payment middleware from the environment. This is
// the only place that knows about concrete providers — adding a new method is a
// new subpackage plus a Register call here, nothing else changes.
func buildPayments() *payment.Registry {
	reg := payment.NewRegistry()
	// Manual is always available (offline / bank transfer, operator-settled).
	reg.Register(manual.New(os.Getenv("NFTVAULT_MANUAL_PAYEE")))

	if sk := os.Getenv("STRIPE_SECRET_KEY"); sk != "" {
		reg.Register(stripe.New(sk, os.Getenv("STRIPE_WEBHOOK_SECRET"), os.Getenv("STRIPE_API_BASE")))
	}
	if addr := os.Getenv("BITCOIN_ADDRESS"); addr != "" {
		minConf, _ := strconv.Atoi(os.Getenv("BITCOIN_MIN_CONF"))
		reg.Register(bitcoin.New(bitcoin.StaticAddress(addr), nil, os.Getenv("BITCOIN_WEBHOOK_SECRET"), minConf))
	}
	return reg
}

func main() {
	listen := flag.String("listen", "127.0.0.1:9000", "address to listen on")
	state := flag.String("state", "control.json", "JSON state file (empty = memory only)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file (recommended)")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	flag.Parse()

	adminToken := os.Getenv("NFTVAULT_ADMIN_TOKEN")
	if adminToken == "" {
		log.Println("WARNING: NFTVAULT_ADMIN_TOKEN is empty — admin routes are LOCKED. Set it to manage plans/accounts.")
	}

	store, err := control.NewStore(*state)
	if err != nil {
		log.Fatalf("state: %v", err)
	}
	payments := buildPayments()
	svc := control.NewService(store, nil, control.ManualProvider{}).WithPayments(payments)
	srv := &http.Server{Addr: *listen, Handler: control.NewAPI(svc, adminToken).Handler()}

	scheme := "http"
	if *tlsCert != "" && *tlsKey != "" {
		scheme = "https"
	}
	log.Printf("vaultserver (control plane) on %s://%s  state=%s  payments=%v", scheme, *listen, *state, payments.Names())
	if scheme == "https" {
		log.Fatal(srv.ListenAndServeTLS(*tlsCert, *tlsKey))
	}
	log.Fatal(srv.ListenAndServe())
}
