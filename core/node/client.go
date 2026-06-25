package node

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nftvault/cas"
)

// RemoteBackend is a cas.Backend backed by a remote node over HTTP(S). A vault
// can mix RemoteBackends and local FSBackends freely as its storage points.
type RemoteBackend struct {
	name    string
	baseURL string
	token   string
	client  *http.Client
}

// NewRemoteBackend targets a node at baseURL (e.g. https://node1.example:8443).
// token must match the node's; pass "" if the node runs without auth.
func NewRemoteBackend(name, baseURL, token string) *RemoteBackend {
	return &RemoteBackend{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *RemoteBackend) Name() string { return r.name }

func (r *RemoteBackend) url(addr string) string {
	return r.baseURL + blockPath + url.PathEscape(addr)
}

func (r *RemoteBackend) do(req *http.Request) (*http.Response, error) {
	req.Header.Set(apiHeader, APIVersion)
	if r.token != "" {
		req.Header.Set(authHeader, authScheme+r.token)
	}
	return r.client.Do(req)
}

func (r *RemoteBackend) Put(addr string, data []byte) error {
	req, err := http.NewRequest(http.MethodPut, r.url(addr), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := r.do(req)
	if err != nil {
		return fmt.Errorf("%s: put: %w", r.name, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: put returned %s", r.name, resp.Status)
	}
	return nil
}

func (r *RemoteBackend) Get(addr string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, r.url(addr), nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: get: %w", r.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, cas.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: get returned %s", r.name, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (r *RemoteBackend) Has(addr string) (bool, error) {
	req, err := http.NewRequest(http.MethodHead, r.url(addr), nil)
	if err != nil {
		return false, err
	}
	resp, err := r.do(req)
	if err != nil {
		return false, fmt.Errorf("%s: has: %w", r.name, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("%s: has returned %s", r.name, resp.Status)
	}
}

// Healthy reports whether the node answers its liveness probe.
func (r *RemoteBackend) Healthy() bool {
	req, err := http.NewRequest(http.MethodGet, r.baseURL+healthPath, nil)
	if err != nil {
		return false
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// Ensure RemoteBackend satisfies cas.Backend at compile time.
var _ cas.Backend = (*RemoteBackend)(nil)
