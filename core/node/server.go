package node

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strings"

	"nftvault/cas"
)

// Server exposes a cas.Backend over HTTP. It is safe for concurrent use.
type Server struct {
	backend cas.Backend
	token   string // bearer token; empty disables auth (local-only use)
	maxBody int64
}

// NewServer wraps a backend. token may be empty for trusted local deployments.
func NewServer(backend cas.Backend, token string) *Server {
	return &Server{backend: backend, token: token, maxBody: 64 << 20} // 64 MiB cap
}

// Handler returns the http.Handler for the node API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthPath, s.health)
	mux.HandleFunc("GET "+blockPath+"{addr}", s.getBlock)
	mux.HandleFunc("HEAD "+blockPath+"{addr}", s.headBlock)
	mux.HandleFunc("PUT "+blockPath+"{addr}", s.putBlock)
	return s.withAPIVersion(mux)
}

func (s *Server) withAPIVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(apiHeader, APIVersion)
		// Health is unauthenticated and version-agnostic for simple probes.
		if r.URL.Path != healthPath {
			if v := r.Header.Get(apiHeader); v != "" && v != APIVersion {
				http.Error(w, "unsupported api version", http.StatusBadRequest)
				return
			}
			if !s.authorized(r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	h := r.Header.Get(authHeader)
	if !strings.HasPrefix(h, authScheme) {
		return false
	}
	got := strings.TrimPrefix(h, authScheme)
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "ok")
}

func (s *Server) getBlock(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("addr")
	data, err := s.backend.Get(addr)
	if errors.Is(err, cas.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "backend error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

func (s *Server) headBlock(w http.ResponseWriter, r *http.Request) {
	ok, err := s.backend.Has(r.PathValue("addr"))
	if err != nil {
		http.Error(w, "backend error", http.StatusInternalServerError)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) putBlock(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("addr")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxBody))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	// Server-side integrity: the bytes MUST hash to the claimed address.
	if cas.Address(data) != addr {
		http.Error(w, "address does not match content", http.StatusBadRequest)
		return
	}
	if err := s.backend.Put(addr, data); err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
