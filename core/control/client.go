package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a control server on behalf of an account. The account's API
// token is supplied per call, so one client can serve many tenants (as a
// storage node does).
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient targets a control server base URL (e.g. https://control:9000).
func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) req(method, path, token string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set(apiHeader, APIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set(authHeader, authScheme+token)
	}
	return c.http.Do(req)
}

// mapStatus turns an HTTP status into a domain error.
func mapStatus(code int, msg string) error {
	switch code {
	case http.StatusInsufficientStorage:
		return ErrQuotaExceeded
	case http.StatusForbidden:
		return ErrSuspended
	case http.StatusUnauthorized:
		return fmt.Errorf("control: unauthorized")
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("control: server returned %d: %s", code, msg)
	}
}

// Reserve accounts for bytes of new storage for the token's account. Returns
// ErrQuotaExceeded or ErrSuspended when the write must be refused.
func (c *Client) Reserve(token string, bytes int64) error {
	resp, err := c.req(http.MethodPost, "/v1/account/usage/reserve", token, map[string]int64{"bytes": bytes})
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return mapStatus(resp.StatusCode, readMsg(resp))
	}
	return nil
}

// Release returns bytes of storage for the token's account.
func (c *Client) Release(token string, bytes int64) error {
	resp, err := c.req(http.MethodPost, "/v1/account/usage/release", token, map[string]int64{"bytes": bytes})
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return mapStatus(resp.StatusCode, readMsg(resp))
	}
	return nil
}

// Status fetches the account's quota/billing view.
func (c *Client) Status(token string) (AccountStatusView, error) {
	resp, err := c.req(http.MethodGet, "/v1/account", token, nil)
	if err != nil {
		return AccountStatusView{}, err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return AccountStatusView{}, mapStatus(resp.StatusCode, readMsg(resp))
	}
	var v AccountStatusView
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return AccountStatusView{}, err
	}
	return v, nil
}

// Healthy reports whether the control server answers its probe.
func (c *Client) Healthy() bool {
	resp, err := c.req(http.MethodGet, "/v1/healthz", "", nil)
	if err != nil {
		return false
	}
	defer drain(resp)
	return resp.StatusCode == http.StatusOK
}

func drain(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func readMsg(resp *http.Response) string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	return e.Error
}
