package control

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// APIVersion is the control-plane wire version.
const (
	APIVersion = "1"
	apiHeader  = "X-Vault-Api"
	authHeader = "Authorization"
	authScheme = "Bearer "
)

// API is the HTTP surface of the control server.
type API struct {
	svc        *Service
	adminToken string
}

// NewAPI wraps a service. adminToken guards the /v1/admin/* routes.
func NewAPI(svc *Service, adminToken string) *API {
	return &API{svc: svc, adminToken: adminToken}
}

// Handler returns the routed, versioned http.Handler.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// Admin routes.
	mux.HandleFunc("POST /v1/admin/plans", a.admin(a.createPlan))
	mux.HandleFunc("GET /v1/admin/plans", a.admin(a.listPlans))
	mux.HandleFunc("POST /v1/admin/accounts", a.admin(a.createAccount))
	mux.HandleFunc("GET /v1/admin/accounts", a.admin(a.listAccounts))
	mux.HandleFunc("POST /v1/admin/accounts/{id}/status", a.admin(a.setStatus))
	mux.HandleFunc("POST /v1/admin/billing/run", a.admin(a.runBilling))
	mux.HandleFunc("POST /v1/admin/billing/enforce", a.admin(a.enforce))
	mux.HandleFunc("POST /v1/admin/invoices/{id}/pay", a.admin(a.payInvoice))

	// Account routes (authenticated by the account's own token).
	mux.HandleFunc("GET /v1/account", a.account(a.accountStatus))
	mux.HandleFunc("GET /v1/account/invoices", a.account(a.accountInvoices))
	mux.HandleFunc("POST /v1/account/usage/reserve", a.account(a.reserve))
	mux.HandleFunc("POST /v1/account/usage/release", a.account(a.release))

	return withAPIVersion(mux)
}

func withAPIVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(apiHeader, APIVersion)
		if v := r.Header.Get(apiHeader); v != "" && v != APIVersion {
			httpError(w, http.StatusBadRequest, "unsupported api version")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get(authHeader)
	if !strings.HasPrefix(h, authScheme) {
		return ""
	}
	return strings.TrimPrefix(h, authScheme)
}

// admin wraps a handler requiring the admin token.
func (a *API) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := bearer(r)
		if a.adminToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(a.adminToken)) != 1 {
			httpError(w, http.StatusUnauthorized, "admin token required")
			return
		}
		h(w, r)
	}
}

// accountCtx carries the authenticated account into account handlers.
type accountHandler func(w http.ResponseWriter, r *http.Request, acc Account)

func (a *API) account(h accountHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc, err := a.svc.Authenticate(bearer(r))
		if err != nil {
			httpError(w, http.StatusUnauthorized, "invalid account token")
			return
		}
		h(w, r, acc)
	}
}

// --- admin handlers --------------------------------------------------------

func (a *API) createPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		QuotaBytes int64  `json:"quota_bytes"`
		PriceCents int64  `json:"price_cents"`
		Currency   string `json:"currency"`
		Period     Period `json:"period"`
	}
	if !decode(w, r, &req) {
		return
	}
	p, err := a.svc.CreatePlan(req.Name, req.QuotaBytes, req.PriceCents, req.Currency, req.Period)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (a *API) listPlans(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, a.svc.ListPlans())
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Email  string `json:"email"`
		PlanID string `json:"plan_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	acc, token, err := a.svc.CreateAccount(req.Name, req.Email, req.PlanID)
	if err != nil {
		writeErr(w, err)
		return
	}
	acc.TokenHash = "" // never echo the hash
	writeJSON(w, http.StatusCreated, map[string]any{"account": acc, "token": token})
}

func (a *API) listAccounts(w http.ResponseWriter, r *http.Request) {
	accs := a.svc.ListAccounts()
	for i := range accs {
		accs[i].TokenHash = ""
	}
	writeJSON(w, 200, accs)
}

func (a *API) setStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status AccountStatus `json:"status"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.SetStatus(r.PathValue("id"), req.Status); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": string(req.Status)})
}

func (a *API) runBilling(w http.ResponseWriter, r *http.Request) {
	issued, err := a.svc.RunBilling()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"issued": issued})
}

func (a *API) enforce(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GraceHours int `json:"grace_hours"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	susp := a.svc.EnforceDelinquency(time.Duration(req.GraceHours) * time.Hour)
	writeJSON(w, 200, map[string]any{"suspended": susp})
}

func (a *API) payInvoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method string `json:"method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	pay, err := a.svc.PayInvoice(r.PathValue("id"), req.Method)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, pay)
}

// --- account handlers ------------------------------------------------------

func (a *API) accountStatus(w http.ResponseWriter, r *http.Request, acc Account) {
	st, err := a.svc.Status(acc.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (a *API) accountInvoices(w http.ResponseWriter, r *http.Request, acc Account) {
	writeJSON(w, 200, a.svc.Invoices(acc.ID))
}

func (a *API) reserve(w http.ResponseWriter, r *http.Request, acc Account) {
	var req struct {
		Bytes int64 `json:"bytes"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.Reserve(acc.ID, req.Bytes); err != nil {
		writeErr(w, err)
		return
	}
	st, _ := a.svc.Status(acc.ID)
	writeJSON(w, 200, st)
}

func (a *API) release(w http.ResponseWriter, r *http.Request, acc Account) {
	var req struct {
		Bytes int64 `json:"bytes"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := a.svc.Release(acc.ID, req.Bytes); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// --- helpers ---------------------------------------------------------------

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeErr maps domain errors to HTTP status codes.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrQuotaExceeded):
		httpError(w, http.StatusInsufficientStorage, err.Error()) // 507
	case errors.Is(err, ErrSuspended):
		httpError(w, http.StatusForbidden, err.Error()) // 403
	case errors.Is(err, ErrNotFound):
		httpError(w, http.StatusNotFound, err.Error()) // 404
	case errors.Is(err, ErrConflict):
		httpError(w, http.StatusConflict, err.Error()) // 409
	case errors.Is(err, ErrInvalid):
		httpError(w, http.StatusBadRequest, err.Error()) // 400
	default:
		httpError(w, http.StatusInternalServerError, err.Error())
	}
}
