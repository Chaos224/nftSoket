package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// snapshot is the full serializable dataset.
type snapshot struct {
	Plans    map[string]Plan      `json:"plans"`
	Accounts map[string]Account   `json:"accounts"`
	Usage    map[string]Usage     `json:"usage"`
	Invoices map[string]Invoice   `json:"invoices"`
	Payments map[string]Payment   `json:"payments"`
}

// Store keeps all control state in memory and, if a path is configured,
// persists an atomic JSON snapshot after every mutation. No external database:
// everything is local and self-owned.
type Store struct {
	mu   sync.RWMutex
	path string
	s    snapshot
}

// NewStore opens a store. An empty path keeps state in memory only; otherwise
// existing state is loaded from path and changes are written back.
func NewStore(path string) (*Store, error) {
	st := &Store{
		path: path,
		s: snapshot{
			Plans:    map[string]Plan{},
			Accounts: map[string]Account{},
			Usage:    map[string]Usage{},
			Invoices: map[string]Invoice{},
			Payments: map[string]Payment{},
		},
	}
	if path == "" {
		return st, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &st.s); err != nil {
		return nil, err
	}
	return st, nil
}

// persist must be called with the write lock held.
func (st *Store) persist() error {
	if st.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

func (st *Store) SavePlan(p Plan) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.Plans[p.ID] = p
	return st.persist()
}

func (st *Store) GetPlan(id string) (Plan, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	p, ok := st.s.Plans[id]
	if !ok {
		return Plan{}, ErrNotFound
	}
	return p, nil
}

func (st *Store) ListPlans() []Plan {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]Plan, 0, len(st.s.Plans))
	for _, p := range st.s.Plans {
		out = append(out, p)
	}
	return out
}

func (st *Store) SaveAccount(a Account) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.Accounts[a.ID] = a
	return st.persist()
}

func (st *Store) GetAccount(id string) (Account, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	a, ok := st.s.Accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return a, nil
}

func (st *Store) GetAccountByTokenHash(h string) (Account, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, a := range st.s.Accounts {
		if a.TokenHash == h {
			return a, nil
		}
	}
	return Account{}, ErrNotFound
}

func (st *Store) ListAccounts() []Account {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]Account, 0, len(st.s.Accounts))
	for _, a := range st.s.Accounts {
		out = append(out, a)
	}
	return out
}

func (st *Store) SaveUsage(u Usage) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.Usage[u.AccountID] = u
	return st.persist()
}

func (st *Store) GetUsage(accountID string) Usage {
	st.mu.RLock()
	defer st.mu.RUnlock()
	u, ok := st.s.Usage[accountID]
	if !ok {
		return Usage{AccountID: accountID}
	}
	return u
}

func (st *Store) SaveInvoice(inv Invoice) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.Invoices[inv.ID] = inv
	return st.persist()
}

func (st *Store) GetInvoice(id string) (Invoice, error) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	inv, ok := st.s.Invoices[id]
	if !ok {
		return Invoice{}, ErrNotFound
	}
	return inv, nil
}

func (st *Store) ListInvoices(accountID string) []Invoice {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]Invoice, 0)
	for _, inv := range st.s.Invoices {
		if accountID == "" || inv.AccountID == accountID {
			out = append(out, inv)
		}
	}
	return out
}

func (st *Store) SavePayment(p Payment) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.s.Payments[p.ID] = p
	return st.persist()
}
