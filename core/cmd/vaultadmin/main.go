// Command vaultadmin is the operator CLI for the control server: define plans,
// create accounts (and get their one-time API token), run billing, suspend or
// reactivate accounts, and settle invoices.
//
// Config: NFTVAULT_ADMIN_TOKEN (required), --server URL or NFTVAULT_CONTROL
// (default http://127.0.0.1:9000).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`vaultadmin — control server operator CLI

  plan-create   --name N --quota SIZE --price CENTS [--currency EUR]
  plan-list
  account-create --name N [--email E] --plan PLAN_ID
  account-list
  account-suspend  --id ACCT_ID
  account-activate --id ACCT_ID
  billing-run
  billing-enforce  --grace-hours 168
  invoice-pay   --id INVOICE_ID [--method manual]

SIZE accepts bytes or K/M/G/T suffixes (e.g. 10G). Set NFTVAULT_ADMIN_TOKEN.
Server: --server URL or NFTVAULT_CONTROL (default http://127.0.0.1:9000).
`)
}

func run(cmd string, args []string) error {
	switch cmd {
	case "plan-create":
		quota, err := parseSize(flagVal(args, "--quota", "0"))
		if err != nil {
			return err
		}
		price, _ := strconv.ParseInt(flagVal(args, "--price", "0"), 10, 64)
		return post("/v1/admin/plans", map[string]any{
			"name":        flagVal(args, "--name", ""),
			"quota_bytes": quota,
			"price_cents": price,
			"currency":    flagVal(args, "--currency", "EUR"),
		})
	case "plan-list":
		return get("/v1/admin/plans")
	case "account-create":
		return post("/v1/admin/accounts", map[string]any{
			"name":    flagVal(args, "--name", ""),
			"email":   flagVal(args, "--email", ""),
			"plan_id": flagVal(args, "--plan", ""),
		})
	case "account-list":
		return get("/v1/admin/accounts")
	case "account-suspend":
		return post("/v1/admin/accounts/"+flagVal(args, "--id", "")+"/status", map[string]any{"status": "suspended"})
	case "account-activate":
		return post("/v1/admin/accounts/"+flagVal(args, "--id", "")+"/status", map[string]any{"status": "active"})
	case "billing-run":
		return post("/v1/admin/billing/run", map[string]any{})
	case "billing-enforce":
		h, _ := strconv.Atoi(flagVal(args, "--grace-hours", "168"))
		return post("/v1/admin/billing/enforce", map[string]any{"grace_hours": h})
	case "invoice-pay":
		return post("/v1/admin/invoices/"+flagVal(args, "--id", "")+"/pay", map[string]any{"method": flagVal(args, "--method", "manual")})
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func base() string {
	if u := os.Getenv("NFTVAULT_CONTROL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:9000"
}

func do(method, path string, body any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base()+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Api", "1")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("NFTVAULT_ADMIN_TOKEN"))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	// Pretty-print JSON responses.
	var pretty bytes.Buffer
	if json.Indent(&pretty, raw, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(raw))
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

func post(path string, body any) error { return do(http.MethodPost, path, body) }
func get(path string) error            { return do(http.MethodGet, path, nil) }

func flagVal(args []string, name, def string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return def
}

// parseSize accepts plain bytes or K/M/G/T (base 1024) suffixes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'K':
		mult, s = 1<<10, s[:len(s)-1]
	case 'M':
		mult, s = 1<<20, s[:len(s)-1]
	case 'G':
		mult, s = 1<<30, s[:len(s)-1]
	case 'T':
		mult, s = 1<<40, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %w", err)
	}
	return n * mult, nil
}
